"""Compare targeted GraphQL batches with WTF's current repository-wide reads.

uv run --no-project python scripts/github-graphql-spike.py --gh /absolute/gh \
  --repo OWNER/NAME=/local/jj/anchor --output /tmp/graphql-results.json

Read-only; requests run serially, one gh process at a time. Private response
bodies and subprocess diagnostics are never printed or persisted in the report.
"""

import argparse
from datetime import datetime, timezone
import json
from pathlib import Path
import re
import subprocess
import tempfile
import time

import importlib.util

spec = importlib.util.spec_from_file_location("fanout_probe", Path(__file__).with_name("github-fanout-spike.py"))
probe = importlib.util.module_from_spec(spec)
spec.loader.exec_module(probe)
META = "number url state isDraft headRefName headRefOid baseRefName isCrossRepository headRepositoryOwner { login }"
CHECKS = "statusCheckRollup { contexts(first:100) { totalCount pageInfo { hasNextPage } nodes { __typename ... on CheckRun { status conclusion } ... on StatusContext { state } } } }"


def run(argv, payload=None):
    """Bound child memory, capture response without pipe buffer deadlocks."""
    initial = probe.machine()
    if initial["MemAvailable"] < 2 * 1024 * 1024:
        raise RuntimeError("insufficient memory headroom")
    with tempfile.TemporaryFile() as incoming, tempfile.TemporaryFile() as outgoing, tempfile.TemporaryFile() as diagnostics:
        incoming.write(json.dumps(payload).encode() if payload else b"")
        incoming.seek(0)
        started = time.monotonic()
        process = subprocess.Popen(argv, stdin=incoming, stdout=outgoing, stderr=diagnostics)
        peak = 0
        try:
            while process.poll() is None:
                if time.monotonic() - started > 20:
                    raise RuntimeError("request exceeded 20 seconds; no retries")
                mem = probe.machine()
                if mem["MemAvailable"] < 1024 * 1024 or initial["SwapFree"] - mem["SwapFree"] > 256 * 1024:
                    raise RuntimeError("resource stop; no retries")
                try:
                    for line in Path(f"/proc/{process.pid}/status").read_text().splitlines():
                        if line.startswith("VmRSS:"):
                            peak = max(peak, int(line.split()[1]))
                except FileNotFoundError:
                    pass
                time.sleep(.02)
            outgoing.seek(0)
            raw = outgoing.read()
            if process.returncode:
                diagnostics.seek(0)
                error = diagnostics.read().lower()
                kind = "throttled" if any(s in error for s in (b"rate limit", b"429", b"abuse")) else "provider failure"
                raise RuntimeError(kind + "; diagnostics withheld; no retries")
            data = json.loads(raw)
            if isinstance(data, dict) and data.get("errors"):
                raise RuntimeError("GraphQL partial failure; no retries")
            return data, {"seconds": round(time.monotonic() - started, 3), "bytes": len(raw), "peak_rss_kib": peak}
        finally:
            if process.poll() is None:
                process.kill()
            process.wait()


def associated(snapshot, prs, owner):
    branches = set(snapshot["bookmarks"])
    for parent in snapshot["parents"]:
        branches.update(parent["bookmarks"])
    result = set()
    for pr in prs:
        exact = snapshot["commit"] == pr["headRefOid"] or any(p["commit"] == pr["headRefOid"] for p in snapshot["parents"])
        branch = pr["headRefName"] in branches and (not pr["isCrossRepository"] or (pr.get("headRepositoryOwner") or {}).get("login", "").lower() == owner.lower())
        if exact or branch:
            result.add(pr["number"])
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--gh", required=True)
    parser.add_argument("--repo", action="append", required=True)
    parser.add_argument("--batch-size", type=int, default=50)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    if not 1 <= args.batch_size <= 50:
        parser.error("batch size must be 1..50")
    repos = {}
    template = re.search(r"const prSnapshotTemplate = `([^`]+)`", Path("internal/core/github.go").read_text()).group(1)
    for specification in args.repo:
        slug, root = specification.split("=", 1)
        if not re.fullmatch(r"[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+", slug):
            parser.error("invalid repository slug")
        native = subprocess.run(["jj", "workspace", "list", "--ignore-working-copy", "-T", template], cwd=root, capture_output=True, timeout=10)
        if native.returncode:
            raise RuntimeError("local JJ inspection failed; diagnostics withheld")
        repos[slug] = [json.loads(line) for line in native.stdout.splitlines() if line]
    measurements = {"discovery": [], "hydration": [], "baseline": []}
    candidates = {slug: {} for slug in repos}
    targets = []
    for slug, snapshots in repos.items():
        unique = set()
        for snapshot in snapshots:
            for revision in [snapshot, *snapshot["parents"]]:
                unique.add(("commit", revision["commit"]))
                unique.update(("branch", name) for name in revision["bookmarks"])
        targets.extend((slug, kind, value) for kind, value in sorted(unique))
    truncated = []
    missing = 0

    def graphql(items, phase):
        fields = []
        aliases = {}
        for index, (slug, kind, value) in enumerate(items):
            owner, name = slug.split("/")
            alias = f"r{index}"
            aliases[alias] = (slug, kind, value)
            if kind == "branch":
                field = f"pullRequests(headRefName:{json.dumps(value)},first:20,orderBy:{{field:UPDATED_AT,direction:DESC}}) {{ pageInfo {{ hasNextPage }} nodes {{ {META} }} }}"
            elif kind == "commit":
                field = f"object(oid:{json.dumps(value)}) {{ ... on Commit {{ associatedPullRequests(first:20) {{ pageInfo {{ hasNextPage }} nodes {{ {META} }} }} }} }}"
            else:
                field = f"pullRequest(number:{value}) {{ {META} {CHECKS} }}"
            fields.append(f"{alias}:repository(owner:{json.dumps(owner)},name:{json.dumps(name)}) {{ {field} }}")
        query = "query { rateLimit { cost remaining used resetAt } " + " ".join(fields) + " }"
        data, metric = run([args.gh, "api", "graphql", "--input", "-"], {"query": query})
        metric.update({"lookups": len(items), "rate_limit": data["data"]["rateLimit"]})
        measurements[phase].append(metric)
        return data["data"], aliases

    for start in range(0, len(targets), args.batch_size):
        data, aliases = graphql(targets[start:start + args.batch_size], "discovery")
        for alias, (slug, kind, value) in aliases.items():
            repository = data[alias]
            if repository is None:
                raise RuntimeError("repository unavailable")
            connection = repository["pullRequests"] if kind == "branch" else (repository["object"] or {}).get("associatedPullRequests")
            if connection is None:
                missing += 1
                continue
            if connection["pageInfo"]["hasNextPage"]:
                truncated.append({"repo": slug, "kind": kind, "target": value})
            for pr in connection["nodes"]:
                candidates[slug][pr["number"]] = pr
    wanted = []
    for slug, snapshots in repos.items():
        numbers = set()
        for snapshot in snapshots:
            numbers.update(associated(snapshot, candidates[slug].values(), slug.split("/")[0]))
        wanted.extend((slug, "number", number) for number in sorted(numbers))
    hydrated = {slug: {} for slug in repos}
    for start in range(0, len(wanted), args.batch_size):
        data, aliases = graphql(wanted[start:start + args.batch_size], "hydration")
        for alias, (slug, _, number) in aliases.items():
            if data[alias] is None or data[alias]["pullRequest"] is None:
                raise RuntimeError("matched PR disappeared during hydration")
            hydrated[slug][number] = data[alias]["pullRequest"]
    comparisons = []
    baseline_total = 0
    baseline_matches = 0
    check_differences = []
    metadata_differences = []
    checks_compared = 0
    for slug, snapshots in repos.items():
        baseline, metric = run([args.gh, "pr", "list", "--repo", slug, "--state", "all", "--limit", "200", "--json", probe.FIELDS])
        metric.update({"repo": slug, "prs": len(baseline), "possibly_truncated": len(baseline) == 200})
        measurements["baseline"].append(metric)
        baseline_total += len(baseline)
        for old_pr in baseline:
            new_pr = hydrated[slug].get(old_pr["number"])
            if new_pr is None:
                continue
            checks_compared += 1
            fields = ("state", "isDraft", "headRefName", "headRefOid", "baseRefName", "isCrossRepository")
            different = [field for field in fields if old_pr.get(field) != new_pr.get(field)]
            if (old_pr.get("headRepositoryOwner") or {}).get("login") != (new_pr.get("headRepositoryOwner") or {}).get("login"):
                different.append("headRepositoryOwner.login")
            if different:
                metadata_differences.append({"repo": slug, "number": old_pr["number"], "fields": different})
            connection = (new_pr.get("statusCheckRollup") or {}).get("contexts", {})
            def normalized(nodes):
                return sorted((node.get("__typename", ""), node.get("status") or "", node.get("conclusion") or "", node.get("state") or "") for node in nodes)
            old_nodes = old_pr.get("statusCheckRollup") or []
            new_nodes = connection.get("nodes") or []
            if normalized(old_nodes) != normalized(new_nodes):
                check_differences.append({"repo": slug, "number": old_pr["number"], "baseline_contexts": len(old_nodes), "targeted_contexts": len(new_nodes)})
        for snapshot in snapshots:
            old = associated(snapshot, baseline, slug.split("/")[0])
            new = associated(snapshot, hydrated[slug].values(), slug.split("/")[0])
            baseline_matches += len(old)
            comparisons.append({"repo": slug, "workspace": snapshot["name"], "baseline": sorted(old), "targeted": sorted(new), "missing_from_targeted": sorted(old - new), "additional_targeted": sorted(new - old)})
    report = {"timestamp": datetime.now(timezone.utc).isoformat(), "workspaces": len(comparisons), "targets": len(targets), "batch_size": args.batch_size,
              "matched_prs_hydrated": len(wanted), "baseline_prs_fetched": baseline_total, "baseline_workspace_matches": baseline_matches,
              "missing_commit_objects": missing, "truncated_discovery": truncated,
              "measurements": measurements, "comparisons": comparisons,
              "matched_prs_compared": checks_compared, "metadata_differences": metadata_differences, "check_differences": check_differences,
              "limitations": ["One live sample, not sustained-load validation", "Baseline is limited to 200 PRs per repo", "Discovery pages limited to 20; truncation is reported, not silently treated as empty", "Local-only commits may have no GitHub object", "Commit association alone does not prove a merged PR has the same head", "No production changes"]}
    args.output.write_text(json.dumps(report, indent=2) + "\n")
    print(json.dumps({k: v for k, v in report.items() if k not in ("comparisons", "limitations")}), flush=True)


if __name__ == "__main__":
    main()

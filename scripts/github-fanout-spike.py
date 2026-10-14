"""Bounded live GHClient workload. Run with uv run --no-project python.

One burst per invocation; no retries, raw provider output, or production changes.
Repeated repository reads deliberately model the current rm picker, not a
deduplicated listing. CLI concurrency is not API request concurrency (pagination).
"""

import argparse
import concurrent.futures
import json
import math
import os
from pathlib import Path
import re
import subprocess
import threading
import time


FIELDS = "number,title,url,state,isDraft,headRefName,headRefOid,baseRefName,headRepositoryOwner,isCrossRepository,statusCheckRollup"


def quota(binary):
    result = subprocess.run(
        [binary, "api", "rate_limit"], capture_output=True, timeout=15
    )
    if result.returncode:
        raise RuntimeError("quota lookup failed; provider output withheld")
    resources = json.loads(result.stdout)["resources"]
    return {key: resources[key] for key in ("core", "graphql", "search")}


def machine():
    mem = {}
    for line in Path("/proc/meminfo").read_text().splitlines():
        key, value = line.split(":", 1)
        if key in ("MemAvailable", "SwapFree"):
            mem[key] = int(value.split()[0])
    return mem


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--gh", required=True)
    parser.add_argument("--repo", action="append", required=True)
    parser.add_argument("--concurrency", type=int, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    if not 1 <= args.concurrency <= 64:
        parser.error("concurrency must be between 1 and 64")
    if any(not re.fullmatch(r"[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+", r) for r in args.repo):
        parser.error("invalid repository slug")
    before = quota(args.gh)
    if before["graphql"]["remaining"] < 1000:
        raise RuntimeError("insufficient quota headroom")
    initial = machine()
    if initial["MemAvailable"] < 2 * 1024 * 1024:
        raise RuntimeError("less than 2 GiB memory headroom")
    stopped = threading.Event()
    resource_stopped = threading.Event()
    lock = threading.Lock()
    children = {}
    peak = {"active_cli": 0, "rss_kib": 0, "live_process_cpu_seconds": 0.0}
    samples = {"min_available_kib": initial["MemAvailable"], "max_swap_used_delta_kib": 0}
    ticks = os.sysconf("SC_CLK_TCK")

    def monitor():
        while not stopped.wait(0.05):
            mem = machine()
            samples["min_available_kib"] = min(samples["min_available_kib"], mem["MemAvailable"])
            swap_delta = initial["SwapFree"] - mem["SwapFree"]
            samples["max_swap_used_delta_kib"] = max(samples["max_swap_used_delta_kib"], swap_delta)
            if mem["MemAvailable"] < 1024 * 1024 or swap_delta > 256 * 1024:
                resource_stopped.set()
                stopped.set()
            with lock:
                pids = list(children)
            rss = 0
            cpu = 0
            for pid in pids:
                try:
                    stat = Path(f"/proc/{pid}/stat").read_text().rsplit(")", 1)[1].split()
                    cpu += (int(stat[11]) + int(stat[12])) / ticks
                    for line in Path(f"/proc/{pid}/status").read_text().splitlines():
                        if line.startswith("VmRSS:"):
                            rss += int(line.split()[1])
                except (FileNotFoundError, ProcessLookupError):
                    pass
            peak["rss_kib"] = max(peak["rss_kib"], rss)
            peak["live_process_cpu_seconds"] = max(peak["live_process_cpu_seconds"], cpu)
            if stopped.is_set():
                with lock:
                    for process in children.values():
                        if process.poll() is None:
                            process.terminate()

    def query(index):
        if stopped.is_set():
            return {"status": "skipped"}
        started = time.monotonic()
        process = subprocess.Popen(
            [args.gh, "pr", "list", "--repo", args.repo[index % len(args.repo)],
             "--state", "all", "--limit", "200", "--json", FIELDS],
            stdout=subprocess.PIPE, stderr=subprocess.PIPE,
        )
        with lock:
            children[process.pid] = process
            peak["active_cli"] = max(peak["active_cli"], len(children))
        try:
            stdout, stderr = process.communicate(timeout=15)
            if process.returncode:
                throttled = any(term in stderr.lower() for term in
                    (b"rate limit", b"429", b"abuse", b"retry-after"))
                status = "throttled" if throttled else (
                    "resource_stop" if resource_stopped.is_set() else "provider_error")
                stopped.set()
            else:
                json.loads(stdout)
                status = "ok"
            return {"status": status, "seconds": round(time.monotonic() - started, 3),
                    "response_bytes": len(stdout)}
        except subprocess.TimeoutExpired:
            process.kill()
            process.communicate()
            stopped.set()
            return {"status": "timeout", "seconds": 15}
        except Exception:
            stopped.set()
            raise
        finally:
            with lock:
                children.pop(process.pid, None)

    sampler = threading.Thread(target=monitor, daemon=True)
    sampler.start()
    started = time.monotonic()
    try:
        with concurrent.futures.ThreadPoolExecutor(max_workers=args.concurrency) as pool:
            results = list(pool.map(query, range(args.concurrency)))
    finally:
        stopped.set()
        sampler.join()
    durations = sorted(r["seconds"] for r in results if "seconds" in r)
    report = {
        "concurrency": args.concurrency,
        "workload": f"one repeated GHClient burst; {len(args.repo)} repositories rotated",
        "wall_seconds": round(time.monotonic() - started, 3),
        "p50_seconds": durations[math.ceil(len(durations) * .5) - 1] if durations else None,
        "p95_seconds": durations[math.ceil(len(durations) * .95) - 1] if durations else None,
        "over_production_8s_budget": sum(d > 8 for d in durations),
        "statuses": {s: sum(r["status"] == s for r in results) for s in sorted({r["status"] for r in results})},
        "peak": peak, "memory": samples,
        "resource_stop": resource_stopped.is_set(),
        "quota_before": before,
        # After failure, make no further provider requests.
        "quota_after": quota(args.gh) if all(r["status"] == "ok" for r in results) else None,
        "results": results,
    }
    args.output.write_text(json.dumps(report, indent=2) + "\n")
    print(json.dumps({k: v for k, v in report.items() if k != "results"}), flush=True)
    return 0 if all(r["status"] == "ok" for r in results) else 1


if __name__ == "__main__":
    raise SystemExit(main())

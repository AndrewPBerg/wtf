#!/usr/bin/env bash
# coverage-check.sh — compute test coverage excluding untestable functions.
#
# Usage: scripts/coverage-check.sh <profile> <min_coverage>
#
# Untestable functions (exec.Command wrappers, signal-based orchestrators,
# integration glue with hardcoded deps) are excluded from the total.

set -euo pipefail

PROFILE="${1:?usage: coverage-check.sh <profile> <min_coverage>}"
MIN="${2:?usage: coverage-check.sh <profile> <min_coverage>}"

# Functions excluded from coverage calculation.
EXCLUDE_FUNCS=(
  ghToken glabToken ghTokenSafe glabTokenSafe tryToken
  RunShell RunInteractive defaultCmdRunner
  runWatchSingle runWatchGlobal watchRepo
  fetchPRMap createCachedForge runLsWithAsyncPRs
  completePRValues completeRemoteBranchValues
  RunPicker RunRepoPicker FetchRefspec
)

# Build function boundary map: file → [(startLine, funcName), ...] sorted by line.
# Then identify line ranges for excluded functions.
# Finally filter the profile and let go tool cover recompute the total.

FUNCLIST=$(go tool cover -func="$PROFILE" | grep -v 'total:')

# Build a sorted list: "file startLine funcName" for all functions.
ALL_FUNCS=$(echo "$FUNCLIST" | awk -F'[\t:]+' '{
    file = $1; line = $2 + 0
    for (i = 3; i <= NF; i++) {
        if ($i != "" && $i !~ /%/) { print file, line, $i; break }
    }
}' | sort -t' ' -k1,1 -k2,2n)

# Build exclude regex from the function list.
EXCL_RE=$(IFS='|'; echo "${EXCLUDE_FUNCS[*]}")

# For each excluded function, find its start line and the next function's start
# line in the same file. Output "file:startLine-endLine" ranges.
RANGES=$(echo "$ALL_FUNCS" | awk -v excl="$EXCL_RE" '
{
    file[NR] = $1; line[NR] = $2; fname[NR] = $3; n = NR
}
END {
    split(excl, ea, "|")
    for (k in ea) exclude[ea[k]] = 1

    for (i = 1; i <= n; i++) {
        if (!(fname[i] in exclude)) continue
        start = line[i]
        end = 999999
        # Find next function in same file
        for (j = i + 1; j <= n; j++) {
            if (file[j] == file[i]) { end = line[j] - 1; break }
        }
        print file[i], start, end
    }
}')

# Filter the coverage profile: remove lines whose file:startLine falls in an excluded range.
FILTERED=$(mktemp)
trap 'rm -f "$FILTERED"' EXIT

head -1 "$PROFILE" > "$FILTERED"  # mode line

tail -n +2 "$PROFILE" | awk -v ranges="$RANGES" '
BEGIN {
    n = split(ranges, lines, "\n")
    for (i = 1; i <= n; i++) {
        split(lines[i], parts, " ")
        rfile[i] = parts[1]; rstart[i] = parts[2] + 0; rend[i] = parts[3] + 0
    }
    nranges = n
}
{
    # Parse file:startLine from profile entry
    idx = index($1, ":")
    file = substr($1, 1, idx - 1)
    rest = substr($1, idx + 1)
    dot = index(rest, ".")
    startline = substr(rest, 1, dot - 1) + 0

    skip = 0
    for (i = 1; i <= nranges; i++) {
        if (file == rfile[i] && startline >= rstart[i] && startline <= rend[i]) {
            skip = 1; break
        }
    }
    if (!skip) print
}' >> "$FILTERED"

# Get adjusted coverage from filtered profile.
RAW=$(go tool cover -func="$PROFILE" | grep 'total:' | awk '{print substr($NF, 1, length($NF)-1)}')
ADJ=$(go tool cover -func="$FILTERED" | grep 'total:' | awk '{print substr($NF, 1, length($NF)-1)}')

echo "Raw coverage (all functions): ${RAW}%"
echo "Adjusted coverage (excluding untestable): ${ADJ}%"

# Compare using awk (no bc needed).
if [ "$(echo "$ADJ $MIN" | awk '{print ($1 < $2)}')" -eq 1 ]; then
  echo "FAIL: adjusted coverage ${ADJ}% is below minimum ${MIN}%"
  exit 1
fi

echo "OK: adjusted coverage ${ADJ}% meets minimum ${MIN}%"

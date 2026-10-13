# Internal metadata API

Commands select the observations they need through `App.Metadata()`. The host
owns repository identity, provider transport, caching, deadlines and account
verification. The same service can later back Lua tables without moving those
responsibilities into hooks.

For a fresh PR, CI and review observation:

```go
rows, err := app.List(ctx, false, false)
if err != nil {
    return err
}
rows = app.Metadata().Observe(ctx, rows, core.MetadataRequest{
    PRs: true,
    CI: true,
    Reviews: true,
})
```

CI and reviews depend on PR association. Selecting either automatically loads
that dependency; callers do not orchestrate separate PR and check requests.
Selecting PRs alone omits checks and review resolutions. An empty request makes
no local identity or provider requests. Raw provider check payloads are omitted
from service results; `PullRequest.Checks` contains the normalized summary.

For a cache-first terminal view:

```go
service := app.Metadata()
request := core.MetadataRequest{PRs: true, CI: true, Reviews: true}
plan := service.Prepare(ctx, rows, request)
preview := plan.Cached
fresh := plan.Refresh(ctx)
```

`Prepare` reads local JJ identity and the PR cache without network requests.
`Refresh` always requests live GitHub state, even when that cache is recent.
It returns independent provider records without changing the preview. Review
links retain their separate cache policy; `ForceReviews: true` bypasses that
cache while preserving workspace/account verification.

The result remains a slice of `WorkspaceView`. Its `GitHub` observation contains
repository identity, lookup status, timestamp, associated PRs and any lookup
error. Checks refer to the published PR head; reviews preserve the GitHub URL
and add a verified destination. Missing associations, unavailable providers and
stale observations remain distinct. Provider failures retain cached evidence.
`pr_state` and `ci_state` use the `MetadataState` enum: `present`, `none`,
`loading`, `unavailable`, or `not_applicable`. These describe presence separately
from `lookup` freshness and `error`. Absent PR and CI observations render as
blank CLI cells, including retained empty observations. Their freshness and
refresh errors remain available in JSON. A failure without an
observation remains unavailable rather than claiming there is no PR or CI.
Existing provider deadlines and cancellation apply; the service does not retry
failed requests. PR work is grouped by registered repository. Duplicate review
URLs in the same repository/account scope resolve once per observation, including
forced refreshes and transient failures; the next observation can retry failures.

## Linear review batches

`Reviews: true` collects uncached exact PR lookups before calling the review
provider. Compatible repositories share a native batch; cache entries remain
owned by each repository and account configuration. Configuration file identity,
profile and expected workspace determine the reuse group. The adapter also checks
the actual selected connection for every distinct repository root.

The native provider runs one Codex app-server at a time. A batch uses one verified
ephemeral context for up to four PRs, or two contexts for larger batches, with
four outstanding reads per context. Both contexts verify the same workspace
before review reads begin. The process closes after the batch; no model turn,
credential extraction, or child diagnostic output is involved.

Providers can implement the typed batch interface directly:

```go
lookups := []core.ReviewLookup{
    {Root: root, URL: "https://github.com/owner/repo/pull/42"},
    {Root: root, URL: "https://github.com/owner/repo/pull/43"},
}
results := (core.NativeReviewClient{}).ResolveBatch(ctx, lookups)
```

Results preserve input order and include a GitHub fallback for each failed or
unavailable lookup. The native adapter accepts at most 200 inputs, retains the
eight-second batch deadline and five-second RPC deadlines, and performs no
automatic retry. Confirmed missing reviews do not stop other lookups. A transient
failure or throttling signal stops queued work while already-running reads finish.
Only verified links and confirmed missing reviews can enter the persistent cache.
Cancellation kills the bridge process group and its app-server on Linux.

`ReviewBatchClient` is optional so existing single-lookup providers remain
compatible. Commands continue to use `MetadataService`; future Lua bindings can
select reviews without knowing the process, context or worker layout.

## Removal inspection

The removal picker first observes its candidate set once with `{PRs: true}`.
It then passes those rows to a repository-bound `RemovalInspector`, which runs
local safety checks and never makes provider requests. Each result can publish
independently while other rows are still being inspected. The preview stays
visible while metadata loads, and pending rows remain unselectable.

```go
inspector, err := app.RemovalInspector(ctx)
if err != nil {
    return err
}
observed := app.Metadata().Observe(ctx, rows, core.MetadataRequest{PRs: true})
for _, row := range observed {
    candidate, err := inspector.Inspect(ctx, row)
    if err != nil {
        return err
    }
    candidates = append(candidates, candidate)
}
```

Removal reuses successful observations within the two-minute metadata lifetime.
The picker binds integration evidence to the exact workspace identity and revision;
deletion revalidates identity and local changes before and after hooks without
refetching provider metadata. Stale or failed observations cannot prove integration.
`InspectRemoval` remains available as a self-contained single-workspace operation.

## Transport and Lua boundary

The workspace browser uses repository-wide `gh pr list` requests. Removal reuses
that observation first. Missing or expired observations use the discovery phase
from `scripts/github-graphql-spike.py`: deduplicated commit and bookmark targets,
up to 50 aliases per request, and at most two pages of 100 PRs per target. Cleanup
omits CI hydration. Partial errors, missing response fields, and exhausted page
bounds remain unavailable observations. Targeted subsets never overwrite the
browser's repository-wide cache. Concurrency and provider deadlines remain bounded.

Lua provider APIs are not exposed by this change. The request fields have stable
JSON names (`prs`, `ci`, `reviews`, `force_reviews`) suitable for a future table
binding. That binding should use this service and existing typed host errors,
preserving ordinary records and account verification. GitHub GraphQL does not
replace the existing host-managed Linear connection.

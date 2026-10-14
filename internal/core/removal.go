package core

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// RemovalCandidate describes local changes and evidence that workspace work has landed.
type RemovalCandidate struct {
	WorkspaceView
	Revision      string   `json:"revision,omitempty"`
	Base          string   `json:"base,omitempty"`
	Integrated    bool     `json:"integrated"`
	Evidence      string   `json:"evidence,omitempty"`
	Ahead         int      `json:"ahead"`
	Changes       []string `json:"changes"`
	Selectable    bool     `json:"selectable"`
	Code          string   `json:"code,omitempty"`
	Warning       string   `json:"warning,omitempty"`
	ObservedEdits bool     `json:"observed_edits"`
	proof         *removalProof
}

// removalProof binds preflight evidence to an exact managed workspace revision.
// It is command-local and never reconstructed from JSON or provider caches.
type removalProof struct {
	id, path, revision string
	integrated         bool
}

// RemovalOptions carries this inspection into deletion without provider reads.
func (r RemovalCandidate) RemovalOptions(force bool) RemovalOptions {
	return RemovalOptions{Force: force, proof: r.proof}
}

// CanRemove keeps destructive intent separate from non-overridable guards.
func (r RemovalCandidate) CanRemove(force bool) bool {
	return r.Selectable || force && r.Code == "workspace_has_changes"
}

// RemovalInspector assesses local cleanup safety using a shared observation.
// Inspection never fetches provider metadata. Actual deletion still revalidates.
type RemovalInspector struct {
	app  *App
	repo Repo
}

// RemovalInspector uses the current repository, or each row's repository when
// invoked from a parent directory, matching List's default scope.
func (a *App) RemovalInspector(ctx context.Context) (*RemovalInspector, error) {
	repo, err := Discover(ctx, a.Cwd)
	if err != nil && ErrorCode(err) != "not_a_jj_repository" {
		return nil, err
	}
	return &RemovalInspector{app: a, repo: repo}, nil
}

// Inspect publishes one independent, bounded local safety assessment.
func (s *RemovalInspector) Inspect(ctx context.Context, row WorkspaceView) (RemovalCandidate, error) {
	bounded, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	repo := s.repo
	if repo.Store == "" {
		var err error
		repo, err = Discover(bounded, row.RepositoryPath)
		if err != nil {
			return RemovalCandidate{}, err
		}
	}
	result := s.app.inspectRemoval(bounded, repo, row)
	if ctx.Err() != nil {
		return result, Fail("command_cancelled", ctx.Err().Error())
	}
	return result, nil
}

// InspectRemoval observes and inspects a single workspace. Multi-row callers
// share Metadata().Observe results with a RemovalInspector instead.
func (a *App) InspectRemoval(ctx context.Context, row WorkspaceView) (RemovalCandidate, error) {
	inspector, err := a.RemovalInspector(ctx)
	if err != nil {
		return RemovalCandidate{}, err
	}
	bounded, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if row.Owned && row.Exists && !samePath(row.Path, a.Cwd) {
		rows := a.RemovalMetadata(bounded, []WorkspaceView{row})
		for _, refreshed := range rows {
			row = refreshed
		}
	}
	return inspector.Inspect(bounded, row)
}

// RemovalCandidates observes active workspaces before an interactive cleanup.
// Normal JJ snapshotting captures on-disk edits; no hooks or removals run here.
func (a *App) RemovalCandidates(ctx context.Context) ([]RemovalCandidate, error) {
	rows, err := a.List(ctx, false, false)
	if err != nil {
		return nil, err
	}
	inspector, err := a.RemovalInspector(ctx)
	if err != nil {
		return nil, err
	}
	active := []WorkspaceView{}
	needsPRs := false
	for _, row := range rows {
		if row.Status != "removed" && row.Exists {
			active = append(active, row)
			needsPRs = needsPRs || row.Owned && !samePath(row.Path, a.Cwd)
		}
	}
	rows = active
	bounded, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if needsPRs {
		rows = a.RemovalMetadata(bounded, rows)
	}
	result := make([]RemovalCandidate, 0, len(rows))
	for _, row := range rows {
		candidate, err := inspector.Inspect(bounded, row)
		if err != nil {
			return result, err
		}
		result = append(result, candidate)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	if ctx.Err() != nil {
		return result, Fail("command_cancelled", ctx.Err().Error())
	}
	return result, nil
}

const removalRevisionTemplate = `"{\"commit\":" ++ json(commit_id) ++ ",\"empty\":" ++ json(empty) ++ ",\"parents\":[" ++ parents.map(|p| "{\"commit\":" ++ json(p.commit_id()) ++ "}").join(",") ++ "]}"`

func removalRevision(ctx context.Context, path string) (prSnapshot, error) {
	value, err := output(ctx, path, "log", "--ignore-working-copy", "--no-graph", "-r", "@", "-T", removalRevisionTemplate)
	var revision prSnapshot
	if err != nil {
		return revision, err
	}
	if err = json.Unmarshal([]byte(value), &revision); err != nil || revision.Commit == "" {
		return revision, Fail("jj_failed", "Cannot inspect workspace revision")
	}
	return revision, nil
}

func removalBase(ctx context.Context, root string) (string, string) {
	for _, name := range []string{"main@origin", "master@origin", "main", "master"} {
		value, err := output(ctx, root, "log", "--ignore-working-copy", "--no-graph", "-r", "present("+name+")", "-T", `commit_id ++ "\n"`)
		commits := strings.Fields(value)
		if err == nil && len(commits) == 1 {
			for _, commit := range commits {
				return name, commit
			}
		}
	}
	return "", ""
}

func (a *App) inspectRemoval(ctx context.Context, r Repo, row WorkspaceView) (result RemovalCandidate) {
	result = RemovalCandidate{WorkspaceView: row, Changes: []string{}}
	defer func() {
		if result.CanRemove(true) && result.Revision != "" {
			result.proof = &removalProof{row.ID, row.Path, result.Revision, result.Integrated}
		}
	}()
	block := func(code, message string) RemovalCandidate {
		result.Code, result.Warning, result.Selectable = code, message, false
		return result
	}
	if samePath(row.Path, r.Root) {
		return block("current_workspace", "Current workspace")
	}
	if !row.Owned {
		return block("protected_workspace", "Adopted or anchor workspace")
	}
	if !row.Exists {
		return block("workspace_unavailable", "Workspace path is unavailable")
	}
	if err := verifyIdentity(ctx, r, row.Workspace); err != nil {
		return block(ErrorCode(err), err.Error())
	}
	before, err := removalRevision(ctx, row.Path)
	if err != nil {
		return block(ErrorCode(err), err.Error())
	}
	baseName, baseCommit := removalBase(ctx, r.Root)
	if _, err = output(ctx, row.Path, "status"); err != nil {
		return block("inspection_failed", "Cannot inspect changes; resolve stale JJ state before cleanup")
	}
	after, err := removalRevision(ctx, row.Path)
	if err != nil {
		return block(ErrorCode(err), err.Error())
	}
	result.Revision = after.Commit
	observed, err := output(ctx, row.Path, "diff", "--ignore-working-copy", "--from", before.Commit, "--to", after.Commit, "--summary")
	if err != nil {
		return block(ErrorCode(err), err.Error())
	}
	result.ObservedEdits = observed != ""
	diff, err := output(ctx, row.Path, "diff", "--ignore-working-copy", "--summary")
	if err != nil {
		return block(ErrorCode(err), err.Error())
	}
	if diff != "" {
		result.Changes = strings.Split(diff, "\n")
	}
	result.Base = baseName
	if baseCommit != "" {
		revisions := []string{after.Commit}
		if after.Empty && len(after.Parents) > 0 {
			revisions = nil
			for _, parent := range after.Parents {
				revisions = append(revisions, parent.Commit)
			}
		}
		outside, lookupErr := output(ctx, r.Root, "log", "--ignore-working-copy", "--no-graph", "-r", "ancestors("+strings.Join(revisions, " | ")+") ~ ancestors("+baseCommit+")", "-T", `commit_id ++ "\n"`)
		if lookupErr == nil {
			result.Ahead = len(strings.Fields(outside))
			result.Integrated = result.Ahead == 0
			if result.Integrated {
				result.Evidence = "jj_ancestry"
			}
		}
	}
	// Reuse a successful recent observation, still requiring an exact published
	// head. Stale observations, branch names and merely closed PRs are not proof.
	if !result.Integrated && recentRemovalMetadata(row.GitHub) {
		changed, diffErr := output(ctx, row.Path, "diff", "--ignore-working-copy", "--from", before.Commit, "--to", after.Commit, "--summary")
		if diffErr == nil && changed == "" {
			for _, pr := range row.GitHub.Matches {
				if pr.State != "MERGED" || pr.Base != "main" && pr.Base != "master" {
					continue
				}
				matched := pr.Head == before.Commit
				if before.Empty {
					// A single empty child of the published head contains no extra work.
					matched = len(before.Parents) == 1
					for _, parent := range before.Parents {
						matched = matched && parent.Commit == pr.Head
					}
				}
				if matched {
					result.Base, result.Integrated, result.Evidence = pr.Base, true, fmt.Sprintf("merged_pr:%d", pr.Number)
					break
				}
			}
		}
	}
	if result.ObservedEdits {
		result.Integrated = false
		result.Evidence = ""
		result.Changes = strings.Split(observed, "\n")
	}
	if result.Integrated {
		result.Changes = []string{} // The current revision's changes are already included.
	}
	if len(result.Changes) > 0 {
		return block("workspace_has_changes", "Changes remain; preserve them before removal")
	}
	result.Selectable = true
	if !result.Integrated {
		result.Warning = "Work is not verified in main/master; review before removal"
	}
	return result
}

func (a *App) removalWithPRs(ctx context.Context, r Repo, w Workspace) RemovalCandidate {
	rows := []WorkspaceView{{Workspace: w, Exists: true}}
	for _, row := range rows {
		candidate := a.inspectRemoval(ctx, r, row)
		if candidate.Integrated || candidate.ObservedEdits || len(candidate.Changes) == 0 {
			return candidate
		}
	}
	rows = a.RemovalMetadata(ctx, rows)
	for _, row := range rows {
		return a.inspectRemoval(ctx, r, row)
	}
	return RemovalCandidate{Code: "inspection_failed"}
}

func recentRemovalMetadata(info *PRInfo) bool {
	if info == nil || info.Error != "" || info.CheckedAt == nil || info.Lookup != "fresh" && info.Lookup != "cached" {
		return false
	}
	age := time.Since(*info.CheckedAt)
	return age >= 0 && age <= prTTL
}

// RemovalMetadata reuses the browser's repository observation. Missing or expired
// observations use targeted discovery, without CI hydration or cache pollution.
func (a *App) RemovalMetadata(ctx context.Context, rows []WorkspaceView) []WorkspaceView {
	plan := a.Metadata().Prepare(ctx, rows, MetadataRequest{PRs: true})
	if !plan.plan.NeedsRefresh {
		return plan.Cached
	}
	if a.PRs == nil {
		targets := map[string][]prSnapshot{}
		for i := range plan.plan.Groups {
			group := &plan.plan.Groups[i]
			group.Transient = true
			for _, row := range rows {
				if row.RepositoryID == group.ID && row.Owned && row.Exists {
					if snapshot, ok := group.Snapshot[row.Name]; ok {
						targets[group.Repository] = append(targets[group.Repository], snapshot)
					}
				}
			}
		}
		plan.prs.Client = removalGHClient{targets: targets}
	}
	return selectMetadata(plan.prs.Refresh(ctx, plan.plan), MetadataRequest{PRs: true})
}

func (a *App) checkRemovalChanges(ctx context.Context, w Workspace, options RemovalOptions) error {
	bounded, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if _, err := output(bounded, w.Path, "status"); err != nil {
		return Fail("inspection_failed", "Cannot inspect changes; resolve stale JJ state before cleanup")
	}
	revision, err := removalRevision(bounded, w.Path)
	if err != nil {
		return err
	}
	if options.Force {
		return nil
	}
	proof := options.proof
	if proof == nil || proof.id != w.ID || !samePath(proof.path, w.Path) {
		return Fail("inspection_failed", "Removal requires an assessment of this workspace")
	}
	if proof.integrated && proof.revision == revision.Commit {
		return nil
	}
	diff, err := output(bounded, w.Path, "diff", "--ignore-working-copy", "--summary")
	if err != nil {
		return err
	}
	if diff != "" {
		return Fail("workspace_has_changes", "Changes remain; inspect the workspace again before removal")
	}
	return nil
}

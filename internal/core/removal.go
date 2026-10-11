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

// RemovalInspector binds cleanup inspection to the current repository.
func (a *App) RemovalInspector(ctx context.Context) (*RemovalInspector, error) {
	repo, err := Discover(ctx, a.Cwd)
	if err != nil {
		return nil, err
	}
	return &RemovalInspector{app: a, repo: repo}, nil
}

// Inspect publishes one independent, bounded local safety assessment.
func (s *RemovalInspector) Inspect(ctx context.Context, row WorkspaceView) (RemovalCandidate, error) {
	bounded, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	result := s.app.inspectRemoval(bounded, s.repo, row)
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
	if row.Owned && row.Exists && !samePath(row.Path, inspector.repo.Root) {
		rows := a.Metadata().Observe(bounded, []WorkspaceView{row}, MetadataRequest{PRs: true})
		for _, refreshed := range rows {
			row = refreshed
		}
	}
	return inspector.Inspect(bounded, row)
}

// RemovalCandidates observes active workspaces before an interactive cleanup.
// Normal JJ snapshotting captures on-disk edits; no hooks or removals run here.
func (a *App) RemovalCandidates(ctx context.Context) ([]RemovalCandidate, error) {
	r, err := Discover(ctx, a.Cwd)
	if err != nil {
		return nil, err
	}
	state, err := r.Load()
	if err != nil {
		return nil, err
	}
	rows := []WorkspaceView{}
	needsPRs := false
	for _, row := range views(state) {
		if row.Status != "removed" {
			rows = append(rows, row)
			needsPRs = needsPRs || row.Owned && row.Exists && !samePath(row.Path, r.Root)
		}
	}
	bounded, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if needsPRs {
		rows = a.Metadata().Observe(bounded, rows, MetadataRequest{PRs: true})
	}
	result := make([]RemovalCandidate, 0, len(rows))
	for _, row := range rows {
		result = append(result, a.inspectRemoval(bounded, r, row))
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

func (a *App) inspectRemoval(ctx context.Context, r Repo, row WorkspaceView) RemovalCandidate {
	result := RemovalCandidate{WorkspaceView: row, Changes: []string{}}
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
	// Only a fresh, exact published head can prove a squash/rebase merge. Branch
	// names, older cached observations and merely closed PRs are not evidence.
	if !result.Integrated && row.GitHub != nil && row.GitHub.Lookup == "fresh" && row.GitHub.Error == "" {
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
	rows = a.Metadata().Observe(ctx, rows, MetadataRequest{PRs: true})
	for _, row := range rows {
		return a.inspectRemoval(ctx, r, row)
	}
	return RemovalCandidate{Code: "inspection_failed"}
}

func (a *App) checkRemovalChanges(ctx context.Context, r Repo, w Workspace, force bool) error {
	bounded, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	assessment := a.removalWithPRs(bounded, r, w)
	if !assessment.CanRemove(force) {
		code := assessment.Code
		if code == "" {
			code = "inspection_failed"
		}
		return Fail(code, assessment.Warning)
	}
	return nil
}

package cli

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/AndrewPBerg/wtf/internal/config"
	"github.com/AndrewPBerg/wtf/internal/identity"
	"github.com/AndrewPBerg/wtf/internal/resource"
	"github.com/AndrewPBerg/wtf/internal/vcs"
	"github.com/spf13/cobra"
)

type resourcesReport struct {
	Version   int                           `json:"version"`
	Resources map[string]resourceReportItem `json:"resources"`
	Findings  []Finding                     `json:"findings,omitempty"`
}

type resourceReportItem struct {
	RepositoryID string                  `json:"repository_id"`
	WorkspaceID  string                  `json:"workspace_id"`
	Desired      resource.Desired        `json:"desired"`
	Observed     []resource.Observed     `json:"observed"`
	Leases       []resource.Lease        `json:"leases"`
	Lifecycle    resource.LifecycleState `json:"lifecycle"`
	CleanupDebt  []resource.CleanupDebt  `json:"cleanup_debt"`
}

var resourcesCmd = &cobra.Command{
	Use:   "resources [workspace-id|name|path]",
	Short: "Inspect declared workspace resources",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		state, err := identity.DefaultStore()
		if err != nil {
			return err
		}
		selectors := args
		if len(selectors) == 0 {
			if workspace, currentErr := currentWorkspace(state); currentErr == nil {
				selectors = []string{workspace.ID}
			}
		}
		items, findings, err := collectResourceReports(state, selectors)
		if err != nil {
			return err
		}
		report := resourcesReport{Version: ReportVersion, Resources: items, Findings: findings}
		if jsonOutput {
			return writeJSON(cmd.OutOrStdout(), report)
		}
		ids := make([]string, 0, len(items))
		for id := range items {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", id, items[id].Lifecycle)
		}
		return nil
	},
}

func init() { rootCmd.AddCommand(resourcesCmd) }

func selectedWorkspaces(state identity.State, args []string) ([]identity.Workspace, error) {
	if len(args) == 1 {
		store, err := identity.DefaultStore()
		if err != nil {
			return nil, err
		}
		w, err := store.LookupWorkspace(args[0])
		if err != nil {
			return nil, err
		}
		return []identity.Workspace{w}, nil
	}
	ws := append([]identity.Workspace(nil), state.Workspaces...)
	sort.Slice(ws, func(i, j int) bool { return ws[i].ID < ws[j].ID })
	return ws, nil
}

func collectResourceReports(store *identity.Store, args []string) (map[string]resourceReportItem, []Finding, error) {
	state, err := store.Load()
	if err != nil {
		return nil, nil, err
	}
	workspaces, err := selectedWorkspaces(state, args)
	if err != nil {
		return nil, nil, err
	}
	repos := make(map[string]identity.Repository)
	for _, r := range state.Repositories {
		repos[r.ID] = r
	}
	out := make(map[string]resourceReportItem)
	var findings []Finding
	for _, w := range workspaces {
		item := resourceReportItem{RepositoryID: w.RepositoryID, WorkspaceID: w.ID, Lifecycle: resource.LifecycleActive,
			Observed: []resource.Observed{}, Leases: []resource.Lease{}, CleanupDebt: []resource.CleanupDebt{}}
		repo, ok := repos[w.RepositoryID]
		if !ok {
			findings = append(findings, finding("identity_repository_missing", SeverityError, w.RepositoryID, w.ID, "workspace references a missing repository"))
			out[w.ID] = item
			continue
		}
		mgr, parseErr := managerForIdentity(w)
		if parseErr != nil {
			findings = append(findings, finding("identity_backend_invalid", SeverityError, w.RepositoryID, w.ID, parseErr.Error()))
			out[w.ID] = item
			continue
		}
		stateDir, dirErr := mgr.StateDir(repo.Locator)
		if dirErr != nil {
			findings = append(findings, finding("vcs_state_unavailable", SeverityError, w.RepositoryID, w.ID, dirErr.Error()))
			out[w.ID] = item
			continue
		}
		reg := resource.NewRegistry(filepath.Join(stateDir, "resources.json"))
		if saved, loadErr := reg.Get(w.ID); loadErr == nil {
			item.Desired, item.Leases, item.Lifecycle, item.CleanupDebt = saved.Desired, saved.Leases, saved.Lifecycle, saved.CleanupDebt
		} else if !strings.Contains(loadErr.Error(), "not found") {
			findings = append(findings, finding("resource_registry_unavailable", SeverityError, w.RepositoryID, w.ID, loadErr.Error()))
		}
		if m, manifestErr := config.LoadManifestFromDir(repo.Locator); manifestErr != nil {
			findings = append(findings, finding("manifest_invalid", SeverityError, w.RepositoryID, w.ID, manifestErr.Error()))
		} else if m != nil {
			if desired, convErr := resource.FromManifest(m); convErr == nil {
				item.Desired = desired
			} else {
				findings = append(findings, finding("manifest_invalid", SeverityError, w.RepositoryID, w.ID, convErr.Error()))
			}
		}
		physical, physicalFindings := observeResources(w, repo, mgr, item.Desired, item.Leases)
		item.Observed = physical
		findings = append(findings, physicalFindings...)
		out[w.ID] = item
	}
	return out, findings, nil
}

func managerForIdentity(w identity.Workspace) (vcs.Manager, error) {
	kind, err := vcs.ParseKind(w.Backend)
	if err != nil {
		return nil, err
	}
	if !vcs.Available(kind) {
		return nil, fmt.Errorf("%s is not available", kind)
	}
	return newManager(kind), nil
}

func finding(code, severity, repoID, workspaceID, message string) Finding {
	return Finding{Code: code, Severity: severity, RepositoryID: repoID, WorkspaceID: workspaceID, Message: message}
}

func observeResources(w identity.Workspace, repo identity.Repository, mgr vcs.Manager, desired resource.Desired, leases []resource.Lease) ([]resource.Observed, []Finding) {
	var observed []resource.Observed
	var findings []Finding
	wts, err := mgr.List(repo.Locator)
	var physical *vcs.Worktree
	if err != nil {
		findings = append(findings, finding("vcs_registration_unavailable", SeverityError, w.RepositoryID, w.ID, err.Error()))
	}
	for i := range wts {
		if filepath.Clean(wts[i].Path) == filepath.Clean(w.Path) || (wts[i].Prunable && wts[i].NativeName == w.NativeName) {
			physical = &wts[i]
			break
		}
	}
	switch {
	case physical == nil:
		findings = append(findings, finding("vcs_registration_missing", SeverityError, w.RepositoryID, w.ID, "workspace is absent from backend registration"))
	case filepath.Clean(physical.Path) != filepath.Clean(w.Path):
		findings = append(findings, finding("identity_path_mismatch", SeverityError, w.RepositoryID, w.ID, "backend registration path disagrees with identity"))
	case physical.NativeName != "" && physical.NativeName != w.NativeName:
		findings = append(findings, finding("vcs_registration_mismatch", SeverityError, w.RepositoryID, w.ID, "backend registration name disagrees with identity"))
	}
	for _, f := range desired.Files {
		target := filepath.Join(w.Path, filepath.FromSlash(f.Target))
		info, statErr := os.Lstat(target)
		o := resource.Observed{Kind: resource.KindFile, Name: f.Name, State: resource.ObservedPresent}
		switch {
		case os.IsNotExist(statErr):
			o.State = resource.ObservedAbsent
			findings = append(findings, finding("managed_file_missing", SeverityWarning, w.RepositoryID, w.ID, fmt.Sprintf("managed file %q is missing", f.Target)))
		case statErr != nil:
			o.State = resource.ObservedUnknown
			o.Detail = statErr.Error()
		case f.Mode == "symlink":
			link, linkErr := os.Readlink(target)
			if linkErr != nil || !sameResourcePath(repo.Locator, f.Source, link) {
				o.State = resource.ObservedInvalid
				findings = append(findings, finding("managed_file_broken", SeverityWarning, w.RepositoryID, w.ID, fmt.Sprintf("managed file %q is not the declared symlink", f.Target)))
			}
		case !info.Mode().IsRegular():
			o.State = resource.ObservedInvalid
			findings = append(findings, finding("managed_file_broken", SeverityWarning, w.RepositoryID, w.ID, fmt.Sprintf("managed file %q is not a regular file", f.Target)))
		}
		observed = append(observed, o)
	}
	for _, p := range desired.Ports {
		o := resource.Observed{Kind: resource.KindPort, Name: p.Name, State: resource.ObservedAbsent}
		acquired := false
		for _, l := range leases {
			if l.Kind == resource.KindPort && l.Name == p.Name && l.State == resource.LeaseAcquired {
				acquired = true
				break
			}
		}
		if !acquired {
			findings = append(findings, finding("port_lease_unavailable", SeverityWarning, w.RepositoryID, w.ID, fmt.Sprintf("port lease %q is not acquired", p.Name)))
			observed = append(observed, o)
			continue
		}
		listener, listenErr := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p.Preferred))
		if listenErr != nil {
			findings = append(findings, finding("port_lease_unavailable", SeverityWarning, w.RepositoryID, w.ID, fmt.Sprintf("preferred port %d for lease %q is unavailable", p.Preferred, p.Name)))
		} else {
			_ = listener.Close()
			o.State = resource.ObservedPresent
		}
		observed = append(observed, o)
	}
	return observed, findings
}

func sameResourcePath(root, source, link string) bool {
	expected := link
	if !filepath.IsAbs(expected) {
		expected = filepath.Join(root, filepath.FromSlash(expected))
	}
	sourcePath := source
	if !filepath.IsAbs(sourcePath) {
		sourcePath = filepath.Join(root, filepath.FromSlash(sourcePath))
	}
	a, e1 := filepath.Abs(filepath.Clean(expected))
	b, e2 := filepath.Abs(filepath.Clean(sourcePath))
	return e1 == nil && e2 == nil && a == b
}

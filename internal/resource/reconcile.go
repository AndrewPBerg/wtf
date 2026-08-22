package resource

import (
	"fmt"
	"sort"
	"strings"
)

// Action is a deterministic reconciliation operation.
type Action string

const (
	// ActionCreate provisions a missing desired resource.
	ActionCreate Action = "create"
	// ActionRemove removes an observed but undesired resource.
	ActionRemove Action = "remove"
	// ActionObserve refreshes metadata-only observation.
	ActionObserve Action = "observe"
)

// PlanItem is one deterministic resource reconciliation operation.
type PlanItem struct {
	Kind   Kind   `json:"kind"`
	Name   string `json:"name"`
	Action Action `json:"action"`
}

// Plan is the complete deterministic resource reconciliation plan.
type Plan struct {
	Items []PlanItem `json:"items"`
}

// Reconcile computes a deterministic plan without touching the filesystem or
// changing the registry. Desired declarations are the source of truth.
func Reconcile(desired Desired, observed []Observed) (Plan, error) {
	if err := validateDesired(desired); err != nil {
		return Plan{}, err
	}
	for _, f := range desired.Files {
		if hasGlob(f.Source) || hasGlob(f.Target) {
			return Plan{}, fmt.Errorf("resource glob patterns are not supported during reconciliation: file %q", f.Name)
		}
	}
	want := make(map[string]Kind)
	for _, f := range desired.Files {
		want[key(KindFile, f.Name)] = KindFile
	}
	for _, p := range desired.Ports {
		want[key(KindPort, p.Name)] = KindPort
	}
	have := make(map[string]Observed)
	for _, o := range observed {
		if err := validateKindName(o.Kind, o.Name); err != nil {
			return Plan{}, err
		}
		have[key(o.Kind, o.Name)] = o
	}
	items := make([]PlanItem, 0)
	for k, kind := range want {
		o, ok := have[k]
		if !ok || o.State != ObservedPresent {
			items = append(items, PlanItem{Kind: kind, Name: resourceName(k), Action: ActionCreate})
		}
	}
	for k, o := range have {
		if _, ok := want[k]; !ok {
			items = append(items, PlanItem{Kind: o.Kind, Name: o.Name, Action: ActionRemove})
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Kind != items[j].Kind {
			return items[i].Kind < items[j].Kind
		}
		return items[i].Name < items[j].Name
	})
	return Plan{Items: items}, nil
}

func hasGlob(value string) bool {
	for _, part := range strings.Split(value, "/") {
		if strings.ContainsAny(part, "*?[") {
			return true
		}
	}
	return false
}

func key(kind Kind, name string) string { return string(kind) + "\x00" + name }
func resourceName(k string) string {
	for i := 0; i < len(k); i++ {
		if k[i] == 0 {
			return k[i+1:]
		}
	}
	return k
}

// Observe atomically replaces observations for a workspace. It is deliberately
// metadata-only: callers provide state, while this package never reads secrets.
func (r *Registry) Observe(id string, observed []Observed) error {
	if err := validateWorkspaceID(id); err != nil {
		return err
	}
	for _, o := range observed {
		if err := validateKindName(o.Kind, o.Name); err != nil {
			return err
		}
		if o.State != ObservedAbsent && o.State != ObservedPresent && o.State != ObservedInvalid && o.State != ObservedUnknown {
			return fmt.Errorf("invalid observed state %q", o.State)
		}
	}
	copyObserved := append([]Observed(nil), observed...)
	sortObserved(copyObserved)
	return r.update(func(all map[string]Workspace) error {
		w, ok := all[id]
		if !ok {
			return fmt.Errorf("workspace %q not found", id)
		}
		w.Observed = copyObserved
		all[id] = w
		return nil
	})
}

// MarkCleanupDebt retains a failed cleanup as visible, repairable state.
func (r *Registry) MarkCleanupDebt(id string, kind Kind, name, reason string) error {
	if err := validateWorkspaceID(id); err != nil {
		return err
	}
	if err := validateKindName(kind, name); err != nil {
		return err
	}
	if reason == "" {
		return fmt.Errorf("cleanup reason must not be empty")
	}
	return r.update(func(all map[string]Workspace) error {
		w, ok := all[id]
		if !ok {
			return fmt.Errorf("workspace %q not found", id)
		}
		for _, d := range w.CleanupDebt {
			if d.Kind == kind && d.Name == name {
				return nil
			}
		}
		w.CleanupDebt = append(w.CleanupDebt, CleanupDebt{Kind: kind, Name: name, Reason: reason})
		sortDebt(w.CleanupDebt)
		w.Lifecycle = LifecycleCleanupFailed
		all[id] = w
		return nil
	})
}

// FinalizeCleanup removes one debt after its physical cleanup has succeeded.
func (r *Registry) FinalizeCleanup(id string, kind Kind, name string) error {
	if err := validateWorkspaceID(id); err != nil {
		return err
	}
	if err := validateKindName(kind, name); err != nil {
		return err
	}
	return r.update(func(all map[string]Workspace) error {
		w, ok := all[id]
		if !ok {
			return fmt.Errorf("workspace %q not found", id)
		}
		found := false
		debt := w.CleanupDebt[:0]
		for _, d := range w.CleanupDebt {
			if d.Kind == kind && d.Name == name {
				found = true
				continue
			}
			debt = append(debt, d)
		}
		if !found {
			return fmt.Errorf("cleanup debt for %s/%s not found", kind, name)
		}
		w.CleanupDebt = debt
		if len(debt) == 0 {
			w.Lifecycle = LifecycleActive
		}
		all[id] = w
		return nil
	})
}

// FinalizeWorkspace records that all resources, including leases, have been
// cleaned up. It is deliberately strict so VCS deletion cannot outrun state.
func (r *Registry) FinalizeWorkspace(id string) error {
	return r.update(func(all map[string]Workspace) error {
		w, ok := all[id]
		if !ok {
			return fmt.Errorf("workspace %q not found", id)
		}
		if len(w.FileOwnership) != 0 || len(w.CleanupDebt) != 0 {
			return fmt.Errorf("workspace %q still has resource cleanup state", id)
		}
		for _, l := range w.Leases {
			if l.State == LeaseAcquired {
				return fmt.Errorf("lease %s/%s is still acquired", l.Kind, l.Name)
			}
		}
		w.Lifecycle = LifecycleFinalized
		all[id] = w
		return nil
	})
}

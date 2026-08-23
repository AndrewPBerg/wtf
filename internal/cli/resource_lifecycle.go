package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/AndrewPBerg/wtf/internal/config"
	"github.com/AndrewPBerg/wtf/internal/resource"
	"github.com/AndrewPBerg/wtf/internal/vcs"
)

func reconcileCreatedResources(id, mainPath, workspacePath, stateDir string) error {
	manifest, err := config.LoadManifestFromDir(mainPath)
	if err != nil || manifest == nil {
		return err
	}
	desired, err := resource.FromManifest(manifest)
	if err != nil {
		return err
	}
	// Reconcile is intentionally called before any registry or filesystem write.
	if _, err := resource.Reconcile(desired, nil); err != nil {
		return err
	}
	reg := resource.NewRegistry(filepath.Join(stateDir, "resources.json"))
	if err := preflightResourceFiles(desired.Files, mainPath, workspacePath, reg, id); err != nil {
		return err
	}
	if err := reg.SetDesired(id, desired); err != nil {
		return err
	}
	// Leases are owned by the workspace UUID, never by a branch or ports.json.
	for _, p := range desired.Ports {
		if _, err := reg.Acquire(id, resource.KindPort, p.Name); err != nil {
			return fmt.Errorf("acquiring port lease %q: %w", p.Name, err)
		}
	}
	for _, f := range desired.Files {
		if err := applyResourceFile(id, f, mainPath, workspacePath, reg); err != nil {
			return err
		}
	}
	return nil
}

func preflightResourceFiles(files []resource.FileIntent, mainPath, workspacePath string, reg *resource.Registry, id string) error {
	saved, _ := reg.Get(id)
	for _, f := range files {
		target := filepath.Join(workspacePath, filepath.FromSlash(f.Target))
		info, err := os.Lstat(target)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("checking resource target %q: %w", f.Target, err)
		}
		owned := fileOwnership(saved, f.Name)
		if f.Mode == "symlink" {
			link, linkErr := os.Readlink(target)
			if linkErr != nil || !sameResourcePath(mainPath, f.Source, link) {
				return fmt.Errorf("refusing to overwrite unmanaged resource target %q", f.Target)
			}
		} else if owned == nil || owned.Mode != "copy" || owned.Target != f.Target {
			return fmt.Errorf("refusing to overwrite unmanaged resource target %q", f.Target)
		} else if !info.Mode().IsRegular() {
			return fmt.Errorf("refusing to overwrite unmanaged resource target %q", f.Target)
		} else if checksum, checksumErr := fileChecksum(target); checksumErr != nil || checksum != owned.Checksum {
			return fmt.Errorf("refusing to overwrite unmanaged resource target %q", f.Target)
		}
		_ = info
	}
	return nil
}

func applyResourceFile(id string, f resource.FileIntent, mainPath, workspacePath string, reg *resource.Registry) error {
	source := filepath.Join(mainPath, filepath.FromSlash(f.Source))
	target := filepath.Join(workspacePath, filepath.FromSlash(f.Target))
	if _, err := os.Lstat(target); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("checking resource target %q: %w", f.Target, err)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("creating resource directory: %w", err)
		}
		if f.Mode == "symlink" {
			if err := os.Symlink(source, target); err != nil {
				return fmt.Errorf("creating resource symlink %q: %w", f.Name, err)
			}
		} else {
			in, err := os.Open(source)
			if err != nil {
				return fmt.Errorf("opening resource source %q: %w", f.Source, err)
			}
			defer func() { _ = in.Close() }()
			out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
			if err != nil {
				return fmt.Errorf("creating resource copy %q: %w", f.Name, err)
			}
			_, copyErr := io.Copy(out, in)
			closeErr := out.Close()
			if copyErr != nil || closeErr != nil {
				_ = os.Remove(target)
				if copyErr != nil {
					return fmt.Errorf("copying resource %q: %w", f.Name, copyErr)
				}
				return fmt.Errorf("closing resource copy %q: %w", f.Name, closeErr)
			}
		}
	}
	ownership := resource.FileOwnership{Name: f.Name, Target: f.Target, Mode: f.Mode}
	if f.Mode == "copy" {
		sum, err := fileChecksum(target)
		if err != nil {
			return err
		}
		ownership.Checksum = sum
	}
	return reg.RecordFileOwnership(id, ownership)
}

func cleanupResources(id, repoDir, workspacePath string, wm vcs.Manager, markFailed func() error) error {
	stateDir, err := wm.StateDir(repoDir)
	if err != nil {
		return err
	}
	reg := resource.NewRegistry(filepath.Join(stateDir, "resources.json"))
	ws, err := reg.Get(id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil // absent manifest/registry preserves zero-config behavior
		}
		return fmt.Errorf("loading resource ownership: %w", err)
	}
	for _, owned := range append([]resource.FileOwnership(nil), ws.FileOwnership...) {
		if err := removeOwnedFile(owned, ws.Desired, repoDir, workspacePath); err != nil {
			debtErr := reg.MarkCleanupDebt(id, resource.KindFile, owned.Name, err.Error())
			identityErr := markFailed()
			return fmt.Errorf("cleaning resource %q: %w (recording debt: %v; recording identity failure: %v)", owned.Name, err, debtErr, identityErr)
		}
		if err := reg.RemoveFileOwnership(id, owned.Name); err != nil {
			debtErr := reg.MarkCleanupDebt(id, resource.KindFile, owned.Name, err.Error())
			identityErr := markFailed()
			return fmt.Errorf("recording cleanup of resource %q: %w (recording debt: %v; recording identity failure: %v)", owned.Name, err, debtErr, identityErr)
		}
		current, getErr := reg.Get(id)
		if getErr != nil {
			return fmt.Errorf("refreshing cleanup state for resource %q: %w", owned.Name, getErr)
		}
		if debtExists(current.CleanupDebt, resource.KindFile, owned.Name) {
			if err := reg.FinalizeCleanup(id, resource.KindFile, owned.Name); err != nil {
				identityErr := markFailed()
				return fmt.Errorf("finalizing cleanup of resource %q: %w (recording identity failure: %v)", owned.Name, err, identityErr)
			}
		}
	}
	// Release every acquired lease, retaining debt if persistence fails.
	for _, lease := range ws.Leases {
		if lease.State != resource.LeaseAcquired {
			continue
		}
		if err := reg.Release(id, lease.Kind, lease.Name, lease.ID); err != nil {
			debtErr := reg.MarkCleanupDebt(id, lease.Kind, lease.Name, err.Error())
			identityErr := markFailed()
			return fmt.Errorf("releasing resource lease %q: %w (recording debt: %v; recording identity failure: %v)", lease.Name, err, debtErr, identityErr)
		}
	}
	if err := reg.FinalizeWorkspace(id); err != nil {
		debtErr := reg.MarkCleanupDebt(id, resource.KindFile, "workspace", err.Error())
		identityErr := markFailed()
		return fmt.Errorf("finalizing resource workspace: %w (recording debt: %v; recording identity failure: %v)", err, debtErr, identityErr)
	}
	return nil
}

func removeOwnedFile(owned resource.FileOwnership, desired resource.Desired, repoDir, workspacePath string) error {
	target := filepath.Join(workspacePath, filepath.FromSlash(owned.Target))
	info, err := os.Lstat(target)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if owned.Mode == "symlink" {
		var source string
		for _, f := range desired.Files {
			if f.Name == owned.Name {
				source = f.Source
				break
			}
		}
		link, err := os.Readlink(target)
		if err != nil || !sameResourcePath(repoDir, source, link) {
			return fmt.Errorf("target %q is no longer the owned symlink", owned.Target)
		}
	} else {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("target %q is no longer the owned copy", owned.Target)
		}
		sum, err := fileChecksum(target)
		if err != nil || sum != owned.Checksum {
			return fmt.Errorf("target %q is no longer the owned copy", owned.Target)
		}
	}
	return os.Remove(target)
}

func debtExists(debt []resource.CleanupDebt, kind resource.Kind, name string) bool {
	for _, item := range debt {
		if item.Kind == kind && item.Name == name {
			return true
		}
	}
	return false
}

func fileOwnership(ws resource.Workspace, name string) *resource.FileOwnership {
	for i := range ws.FileOwnership {
		if ws.FileOwnership[i].Name == name {
			return &ws.FileOwnership[i]
		}
	}
	return nil
}
func fileChecksum(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

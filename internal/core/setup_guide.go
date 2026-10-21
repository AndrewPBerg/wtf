package core

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
)

const guideChecksumFile = ".agents-template.sha256"

func guideChecksum(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// refreshWorkspaceGuide preserves unknown provenance and local edits. The
// checksum always describes a shipped template, never the customized file.
func refreshWorkspaceGuide(ctx context.Context, path string, template []byte, result *SetupResult) error {
	root, err := os.OpenRoot(path)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	// Separate setup invocations must observe a consistent guide/checksum pair.
	if info, err := root.Lstat(".setup.lock"); err == nil && !info.Mode().IsRegular() {
		return Fail("invalid_setup_path", "Expected a regular setup lock")
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	unlock, err := lock(ctx, filepath.Join(path, ".setup.lock"))
	if err != nil {
		return err
	}
	defer unlock()
	rt := &Runtime{Context: ctx}
	read := func(name string) ([]byte, bool, error) {
		info, err := root.Lstat(name)
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		if err != nil {
			return nil, false, err
		}
		if !info.Mode().IsRegular() {
			return nil, true, Fail("invalid_setup_path", "Expected a regular file, not a directory or symlink: .wtf/"+name)
		}
		data, err := rt.readFile(root, name)
		return data, true, err
	}
	record, recorded, err := read(guideChecksumFile)
	if err != nil {
		return err
	}
	previous := strings.TrimSpace(string(record))
	if recorded {
		decoded, err := hex.DecodeString(previous)
		if err != nil || len(decoded) != sha256.Size {
			return Fail("invalid_setup_state", "Invalid .wtf/"+guideChecksumFile+"; restore the template checksum or remove it to preserve the guide as untracked")
		}
		previous = strings.ToLower(previous)
	}
	current, exists, err := read("AGENTS.md")
	if err != nil {
		return err
	}
	latest := guideChecksum(template)
	if exists && !bytes.Equal(current, template) && (!recorded || guideChecksum(current) != previous) {
		result.Existing = append(result.Existing, ".wtf/AGENTS.md")
		result.UpdateAvailable = append(result.UpdateAvailable, ".wtf/AGENTS.md")
		if recorded {
			result.Existing = append(result.Existing, ".wtf/"+guideChecksumFile)
		}
		return nil
	}
	if !exists || !bytes.Equal(current, template) {
		outcome, err := rt.writeFile(root, "AGENTS.md", template, exists)
		if err != nil {
			return err
		}
		if outcome.Status != "written" {
			// An external writer created a file after inspection. Never enroll it.
			result.Existing = append(result.Existing, ".wtf/AGENTS.md")
			result.UpdateAvailable = append(result.UpdateAvailable, ".wtf/AGENTS.md")
			return nil
		}
		if exists {
			result.Updated = append(result.Updated, ".wtf/AGENTS.md")
		} else {
			result.Created = append(result.Created, ".wtf/AGENTS.md")
		}
	} else {
		result.Existing = append(result.Existing, ".wtf/AGENTS.md")
	}
	if !recorded || previous != latest {
		if _, err := rt.writeFile(root, guideChecksumFile, []byte(latest+"\n"), recorded); err != nil {
			return err
		}
		if recorded {
			result.Updated = append(result.Updated, ".wtf/"+guideChecksumFile)
		} else {
			result.Created = append(result.Created, ".wtf/"+guideChecksumFile)
		}
	} else {
		result.Existing = append(result.Existing, ".wtf/"+guideChecksumFile)
	}
	return nil
}

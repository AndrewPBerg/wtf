package vcs

import (
	"os"
	"os/exec"
	"path/filepath"
)

// Detection holds what was found by walking up from a starting directory.
type Detection struct {
	// Root is the directory holding the marker(s) — the current checkout's root.
	Root string
	// Kinds lists the backends present at Root, git first when both are.
	Kinds []Kind
}

// Has reports whether the given backend was detected.
func (d Detection) Has(k Kind) bool {
	for _, got := range d.Kinds {
		if got == k {
			return true
		}
	}
	return false
}

// Colocated reports whether both backends share the same root — the case where
// wtf cannot infer intent and has to ask.
func (d Detection) Colocated() bool {
	return d.Has(KindGit) && d.Has(KindJJ)
}

// Detect walks up from start looking for a .git or .jj marker and returns what
// it found at the first directory containing either.
//
// Stopping at the *first* level that matches is deliberate. A non-main jj
// workspace has a .jj directory but no .git at all, so continuing upward could
// find an unrelated ancestor git repo and wrongly report the workspace as
// colocated. Only markers sharing a directory mean colocation.
func Detect(start string) (Detection, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return Detection{}, err
	}

	for {
		var kinds []Kind
		// .git is a directory in a primary checkout and a file in a git
		// worktree; both are valid markers.
		if _, err := os.Lstat(filepath.Join(dir, ".git")); err == nil {
			kinds = append(kinds, KindGit)
		}
		if info, err := os.Lstat(filepath.Join(dir, ".jj")); err == nil && info.IsDir() {
			kinds = append(kinds, KindJJ)
		}
		if len(kinds) > 0 {
			return Detection{Root: dir, Kinds: kinds}, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return Detection{}, ErrNotARepo
		}
		dir = parent
	}
}

// binaryAvailable reports whether a backend's CLI is on PATH. Declared as a var
// so tests can simulate a missing binary.
var binaryAvailable = func(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// Available reports whether the CLI backing the given kind is installed.
func Available(k Kind) bool {
	switch k {
	case KindGit:
		return binaryAvailable("git")
	case KindJJ:
		return binaryAvailable("jj")
	default:
		return false
	}
}

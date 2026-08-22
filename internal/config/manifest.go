package config

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/pelletier/go-toml/v2"
)

// Manifest is the versioned, declarative project resource manifest.
// It contains resource metadata only; it never contains or reads secret values.
type Manifest struct {
	Version   int
	Workspace Workspace
	Resources Resources
}

// Workspace contains workspace policy defaults.
type Workspace struct {
	DefaultWorkspace string
}

// Resources contains the typed resource declarations.
type Resources struct {
	// Ports is sorted by Name for deterministic consumers.
	Ports []Port
	// Files retain TOML declaration order.
	Files []File
}

// Port declares a preferred port by name.
type Port struct {
	Name      string
	Preferred int
}

// File declares metadata for a managed file resource.
type File struct {
	Name   string
	Source string
	Target string
	Mode   string
	Secret bool
}

type manifestTOML struct {
	Version   *int           `toml:"version"`
	Workspace *workspaceTOML `toml:"workspace"`
	Resources *resourcesTOML `toml:"resources"`
}

type workspaceTOML struct {
	DefaultWorkspace *string `toml:"default_workspace"`
}

type resourcesTOML struct {
	Ports map[string]portTOML `toml:"ports"`
	Files []fileTOML          `toml:"files"`
}

type portTOML struct {
	Preferred *int `toml:"preferred"`
}

type fileTOML struct {
	Name   *string `toml:"name"`
	Source *string `toml:"source"`
	Target *string `toml:"target"`
	Mode   *string `toml:"mode"`
	Secret *bool   `toml:"secret"`
}

// ParseManifest parses and strictly validates a v1 .wtf.toml manifest.
func ParseManifest(data []byte) (*Manifest, error) {
	var raw manifestTOML
	decoder := toml.NewDecoder(bytes.NewReader(data)).DisallowUnknownFields()
	if err := decoder.Decode(&raw); err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}
	if raw.Version == nil || *raw.Version != 1 {
		return nil, fmt.Errorf("manifest version must be 1")
	}

	manifest := &Manifest{Version: 1}
	if raw.Workspace != nil {
		if raw.Workspace.DefaultWorkspace == nil {
			return nil, fmt.Errorf("workspace.default_workspace is required")
		}
		if !validWorkspacePolicy(*raw.Workspace.DefaultWorkspace) {
			return nil, fmt.Errorf("workspace.default_workspace must be allow, warn, or deny")
		}
		manifest.Workspace.DefaultWorkspace = *raw.Workspace.DefaultWorkspace
	}
	if raw.Resources == nil {
		return manifest, nil
	}

	portNames := make([]string, 0, len(raw.Resources.Ports))
	for name := range raw.Resources.Ports {
		portNames = append(portNames, name)
	}
	sort.Strings(portNames)
	for _, name := range portNames {
		port := raw.Resources.Ports[name]
		if port.Preferred == nil {
			return nil, fmt.Errorf("resources.ports.%s.preferred is required", name)
		}
		if *port.Preferred < 1 || *port.Preferred > 65535 {
			return nil, fmt.Errorf("resources.ports.%s.preferred must be between 1 and 65535", name)
		}
		manifest.Resources.Ports = append(manifest.Resources.Ports, Port{Name: name, Preferred: *port.Preferred})
	}

	seenFiles := make(map[string]struct{}, len(raw.Resources.Files))
	for i, file := range raw.Resources.Files {
		if file.Name == nil || file.Source == nil || file.Target == nil || file.Mode == nil || file.Secret == nil {
			return nil, fmt.Errorf("resources.files[%d] requires name, source, target, mode, and secret", i)
		}
		if *file.Name == "" {
			return nil, fmt.Errorf("resources.files[%d].name must not be empty", i)
		}
		if _, exists := seenFiles[*file.Name]; exists {
			return nil, fmt.Errorf("resources.files[%d].name %q is duplicated", i, *file.Name)
		}
		seenFiles[*file.Name] = struct{}{}
		if *file.Mode != "symlink" && *file.Mode != "copy" {
			return nil, fmt.Errorf("resources.files[%d].mode must be symlink or copy", i)
		}
		if err := validateManifestPath(*file.Source); err != nil {
			return nil, fmt.Errorf("resources.files[%d].source: %w", i, err)
		}
		if err := validateManifestPath(*file.Target); err != nil {
			return nil, fmt.Errorf("resources.files[%d].target: %w", i, err)
		}
		manifest.Resources.Files = append(manifest.Resources.Files, File{
			Name: *file.Name, Source: *file.Source, Target: *file.Target, Mode: *file.Mode, Secret: *file.Secret,
		})
	}
	return manifest, nil
}

// LoadManifest reads a manifest file. A missing file returns (nil, nil), preserving
// WTF's zero-config behavior.
func LoadManifest(filename string) (*Manifest, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading manifest: %w", err)
	}
	manifest, err := ParseManifest(data)
	if err != nil {
		return nil, fmt.Errorf("loading %s: %w", filename, err)
	}
	return manifest, nil
}

// LoadManifestFromDir loads .wtf.toml from dir, returning nil when it is absent.
func LoadManifestFromDir(dir string) (*Manifest, error) {
	return LoadManifest(filepath.Join(dir, ".wtf.toml"))
}

func validWorkspacePolicy(policy string) bool {
	return policy == "allow" || policy == "warn" || policy == "deny"
}

func validateManifestPath(value string) error {
	if value == "" {
		return fmt.Errorf("path must not be empty")
	}
	if len(value) > 4096 || strings.Contains(value, "\\") || strings.HasPrefix(value, "/") || strings.HasPrefix(value, "~") {
		return fmt.Errorf("must be a bounded relative path or glob")
	}
	if strings.IndexFunc(value, func(r rune) bool { return r == 0 || unicode.IsControl(r) }) >= 0 {
		return fmt.Errorf("contains an invalid character")
	}
	parts := strings.Split(value, "/")
	if len(parts) > 64 {
		return fmt.Errorf("has too many path segments")
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return fmt.Errorf("must not contain empty, . or .. path segments")
		}
		if _, err := path.Match(part, ""); err != nil {
			return fmt.Errorf("contains an invalid glob: %w", err)
		}
	}
	return nil
}

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

const documentedManifest = `version = 1

[workspace]
default_workspace = "warn" # allow | warn | deny

[resources.ports.web]
preferred = 3000

[[resources.files]]
name = "root-env"
source = ".env"
target = ".env"
mode = "symlink" # symlink | copy
secret = true

[[resources.files]]
name = "agent-notes"
source = "docs/agent-context.md"
target = "AGENT_CONTEXT.md"
mode = "symlink"
secret = false
`

func TestParseManifestDocumentedSample(t *testing.T) {
	manifest, err := ParseManifest([]byte(documentedManifest))
	require.NoError(t, err)
	require.Equal(t, 1, manifest.Version)
	require.Equal(t, "warn", manifest.Workspace.DefaultWorkspace)
	require.Equal(t, []Port{{Name: "web", Preferred: 3000}}, manifest.Resources.Ports)
	require.Equal(t, []File{
		{Name: "root-env", Source: ".env", Target: ".env", Mode: "symlink", Secret: true},
		{Name: "agent-notes", Source: "docs/agent-context.md", Target: "AGENT_CONTEXT.md", Mode: "symlink", Secret: false},
	}, manifest.Resources.Files)
}

func TestParseManifestSortsPortsAndPreservesFiles(t *testing.T) {
	manifest, err := ParseManifest([]byte(`version = 1
[resources.ports.z]
preferred = 2
[resources.ports.a]
preferred = 1
[[resources.files]]
name = "z"
source = "z.md"
target = "z.md"
mode = "copy"
secret = false
[[resources.files]]
name = "a"
source = "a.md"
target = "a.md"
mode = "copy"
secret = false
`))
	require.NoError(t, err)
	require.Equal(t, []Port{{Name: "a", Preferred: 1}, {Name: "z", Preferred: 2}}, manifest.Resources.Ports)
	require.Equal(t, "z", manifest.Resources.Files[0].Name)
}

func TestParseManifestRejectsUnknownKeysAtEveryLevel(t *testing.T) {
	cases := []string{
		"extra = true",
		"version = 1\n[workspace]\ndefault_workspace = \"allow\"\nextra = true",
		"version = 1\n[resources]\nextra = true",
		"version = 1\n[resources.ports.web]\npreferred = 1\nextra = true",
		"version = 1\n[[resources.files]]\nname=\"x\"\nsource=\"x\"\ntarget=\"x\"\nmode=\"copy\"\nsecret=false\nextra=true",
	}
	for _, input := range cases {
		t.Run(input, func(t *testing.T) {
			_, err := ParseManifest([]byte(input))
			require.Error(t, err)
		})
	}
}

func TestParseManifestRejectsInvalidValuesAndPaths(t *testing.T) {
	cases := []string{
		"",
		"version = 2",
		"version = 1\n[workspace]\ndefault_workspace = \"maybe\"",
		"version = 1\n[resources.ports.web]",
		"version = 1\n[resources.ports.web]\npreferred = 0",
		"version = 1\n[resources.ports.web]\npreferred = 65536",
		"version = 1\n[[resources.files]]\nname=\"x\"\nsource=\"/etc/x\"\ntarget=\"x\"\nmode=\"copy\"\nsecret=false",
		"version = 1\n[[resources.files]]\nname=\"x\"\nsource=\"a/../x\"\ntarget=\"x\"\nmode=\"copy\"\nsecret=false",
		"version = 1\n[[resources.files]]\nname=\"x\"\nsource=\"a\\\\x\"\ntarget=\"x\"\nmode=\"copy\"\nsecret=false",
		"version = 1\n[[resources.files]]\nname=\"x\"\nsource=\"[bad\"\ntarget=\"x\"\nmode=\"copy\"\nsecret=false",
		"version = 1\n[[resources.files]]\nname=\"x\"\nsource=\"x\"\ntarget=\"x\"\nmode=\"copy\"",
		"version = 1\n[[resources.files]]\nname=\"x\"\nsource=\"x\"\ntarget=\"x\"\nmode=\"copy\"\nsecret=false\n[[resources.files]]\nname=\"x\"\nsource=\"y\"\ntarget=\"y\"\nmode=\"copy\"\nsecret=false",
	}
	for _, input := range cases {
		t.Run(input, func(t *testing.T) {
			_, err := ParseManifest([]byte(input))
			require.Error(t, err)
		})
	}
}

func TestLoadManifestMissingPreservesZeroConfig(t *testing.T) {
	manifest, err := LoadManifest(filepath.Join(t.TempDir(), ".wtf.toml"))
	require.NoError(t, err)
	require.Nil(t, manifest)
}

func TestLoadManifest(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(dir, ".wtf.toml")
	require.NoError(t, os.WriteFile(filename, []byte(documentedManifest), 0o644))
	manifest, err := LoadManifestFromDir(dir)
	require.NoError(t, err)
	require.NotNil(t, manifest)
	require.Equal(t, "root-env", manifest.Resources.Files[0].Name)
}

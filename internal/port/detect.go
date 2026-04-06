package port

import (
	"os"
	"path/filepath"
)

// DefaultBasePort is used when no framework is detected.
const DefaultBasePort = 8000

// Framework describes a detected project framework.
type Framework struct {
	Name     string // human-readable name
	BasePort int    // default dev-server port
	DevCmd   string // dev-server command (may contain $PORT placeholder)
}

// frameworkDef maps an indicator file glob to a framework.
type frameworkDef struct {
	Glob      string
	Framework Framework
}

// frameworkDefs is checked in priority order; first match wins.
var frameworkDefs = []frameworkDef{
	// JS meta-frameworks (check before generic vite/svelte)
	{Glob: "next.config.*", Framework: Framework{"Next.js", 3000, "npx next dev -p $PORT"}},
	{Glob: "nuxt.config.*", Framework: Framework{"Nuxt", 3000, "npx nuxi dev --port $PORT"}},
	{Glob: "remix.config.*", Framework: Framework{"Remix", 3000, "npx remix dev --port $PORT"}},
	{Glob: "astro.config.*", Framework: Framework{"Astro", 4321, "npx astro dev --port $PORT"}},
	{Glob: "angular.json", Framework: Framework{"Angular", 4200, "npx ng serve --port $PORT"}},
	// Vite / SvelteKit
	{Glob: "vite.config.*", Framework: Framework{"Vite", 5173, "npx vite --port $PORT"}},
	{Glob: "svelte.config.*", Framework: Framework{"SvelteKit", 5173, "npx vite --port $PORT"}},
	// Python
	{Glob: "manage.py", Framework: Framework{"Django", 8000, "python manage.py runserver 0.0.0.0:$PORT"}},
	// Go
	{Glob: "go.mod", Framework: Framework{"Go", 8080, "go run ."}},
	// Ruby on Rails
	{Glob: "Gemfile", Framework: Framework{"Rails", 3000, "bundle exec rails server -p $PORT"}},
	// Elixir/Phoenix
	{Glob: "mix.exs", Framework: Framework{"Phoenix", 4000, "mix phx.server"}},
}

// DetectFramework returns the project framework for dir, or nil if unknown.
func DetectFramework(dir string) *Framework {
	for _, fd := range frameworkDefs {
		pattern := filepath.Join(dir, fd.Glob)
		matches, err := filepath.Glob(pattern)
		if err != nil {
			continue
		}
		for _, m := range matches {
			if info, sErr := os.Lstat(m); sErr == nil && !info.IsDir() {
				f := fd.Framework // copy
				return &f
			}
		}
	}
	return nil
}

// DetectBasePort returns the default dev-server port for the project in dir.
// Returns DefaultBasePort if no known framework is detected.
func DetectBasePort(dir string) int {
	if fw := DetectFramework(dir); fw != nil {
		return fw.BasePort
	}
	return DefaultBasePort
}

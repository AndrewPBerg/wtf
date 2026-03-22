package setup

import (
	"os"
	"path/filepath"
)

// PackageManager describes a detected package manager.
type PackageManager struct {
	Name       string
	InstallCmd string
	Lockfile   string
}

// packageManagerDefs lists package managers in priority order.
var packageManagerDefs = []PackageManager{
	// JS/TS
	{Name: "pnpm", InstallCmd: "pnpm install", Lockfile: "pnpm-lock.yaml"},
	{Name: "bun", InstallCmd: "bun install", Lockfile: "bun.lockb"},
	{Name: "yarn", InstallCmd: "yarn install", Lockfile: "yarn.lock"},
	{Name: "npm", InstallCmd: "npm install", Lockfile: "package-lock.json"},
	// Python
	{Name: "uv", InstallCmd: "uv sync", Lockfile: "uv.lock"},
	{Name: "poetry", InstallCmd: "poetry install", Lockfile: "poetry.lock"},
	{Name: "pip", InstallCmd: "pip install -r requirements.txt", Lockfile: "requirements.txt"},
	{Name: "uv", InstallCmd: "uv sync", Lockfile: "pyproject.toml"},
	// Go
	{Name: "go", InstallCmd: "go mod download", Lockfile: "go.sum"},
	// Rust
	{Name: "cargo", InstallCmd: "cargo build", Lockfile: "Cargo.lock"},
	// Ruby
	{Name: "bundler", InstallCmd: "bundle install", Lockfile: "Gemfile.lock"},
	// PHP
	{Name: "composer", InstallCmd: "composer install", Lockfile: "composer.lock"},
	// Java/Kotlin
	{Name: "maven", InstallCmd: "mvn install", Lockfile: "pom.xml"},
	{Name: "gradle", InstallCmd: "gradle build", Lockfile: "build.gradle"},
	{Name: "gradle", InstallCmd: "gradle build", Lockfile: "build.gradle.kts"},
	// .NET
	{Name: "dotnet", InstallCmd: "dotnet restore", Lockfile: "packages.lock.json"},
	// Elixir
	{Name: "mix", InstallCmd: "mix deps.get", Lockfile: "mix.lock"},
	// Swift
	{Name: "swift", InstallCmd: "swift package resolve", Lockfile: "Package.resolved"},
}

// DetectPackageManager detects the package manager from lockfiles in dir.
// Returns nil, nil if no known lockfile is found.
func DetectPackageManager(dir string) (*PackageManager, error) {
	for _, pm := range packageManagerDefs {
		path := filepath.Join(dir, pm.Lockfile)
		if _, err := os.Stat(path); err == nil {
			result := pm // copy
			return &result, nil
		}
	}
	return nil, nil
}

// Package walkutil provides shared filesystem-walk helpers that avoid
// descending into common build, dependency, and cache directories.
//
// These helpers exist because recursive operations (config discovery,
// markdown scanning, MCP project search) were independently re-implementing
// skip lists — and at least one of them (recursive `generate`) was missing
// any skip logic, causing minutes-long walks through directories like
// `target/`, `node_modules/`, and `.venv/` and fatal failures on stale
// build-artifact symlinks.
package walkutil

// SkipDirs is the set of directory base names that should never be descended
// into during recursive operations. Matching is by base name and applies at
// any depth in the tree.
//
// Includes VCS metadata, IDE/editor caches, language toolchain caches, and
// build/dependency output directories for the common ecosystems we encounter.
var SkipDirs = map[string]struct{}{
	// VCS metadata
	".git": {}, ".hg": {}, ".svn": {},

	// Editors / IDEs / OS
	".idea": {}, ".vscode": {}, ".vs": {}, ".fleet": {}, ".zed": {},
	".DS_Store": {},

	// Generic caches / temp
	".cache": {}, ".tmp": {}, "tmp": {}, "temp": {}, "logs": {},

	// Rust
	"target": {},

	// Node / JS / TS
	"node_modules": {}, "bower_components": {}, ".yarn": {}, ".pnpm-store": {},
	".next": {}, ".nuxt": {}, ".svelte-kit": {}, ".turbo": {}, ".parcel-cache": {},
	".rollup.cache": {}, ".vite": {}, ".astro": {},

	// Python
	".venv": {}, "venv": {}, ".env": {}, "__pycache__": {},
	".pytest_cache": {}, ".mypy_cache": {}, ".ruff_cache": {},
	".tox": {}, ".nox": {}, ".eggs": {}, ".hypothesis": {},

	// Go
	"vendor": {},

	// Java / JVM / Android
	".gradle": {}, ".mvn": {}, ".m2": {},

	// .NET
	"obj": {},

	// Generic build / output
	"dist": {}, "build": {}, "out": {}, "bin": {},

	// Coverage / test artifacts
	"coverage": {}, ".nyc_output": {}, "htmlcov": {},

	// Infra / cloud caches
	".terraform": {}, ".serverless": {}, ".aws-sam": {},

	// Misc
	".bundle": {}, ".sass-cache": {},
}

// preserve lists hidden directories that should be descended into despite
// the "skip dotfile" rule below. `.ai-rulez` is the whole point of the walk;
// `.github` commonly hosts content the user wants discovered.
var preserve = map[string]struct{}{
	".ai-rulez": {},
	".github":   {},
}

// ShouldSkipDir reports whether a directory with the given base name should
// be skipped during a recursive walk.
//
// It returns true for:
//   - Names in [SkipDirs].
//   - Hidden directories (names starting with ".") not in the preserve list.
//
// The root entry "." is never skipped.
func ShouldSkipDir(name string) bool {
	if name == "" || name == "." {
		return false
	}
	if _, ok := SkipDirs[name]; ok {
		return true
	}
	if len(name) > 1 && name[0] == '.' {
		if _, ok := preserve[name]; ok {
			return false
		}
		return true
	}
	return false
}

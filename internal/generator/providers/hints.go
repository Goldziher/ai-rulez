package providers

import (
	"path"
	"slices"
	"sort"
	"strings"
	"sync"
)

// This file derives registry facts from the embedded provider specs so that a new
// builtin/*.toml needs no hand-maintained list elsewhere: the preset name for
// config validation, the root-file owner table for target filtering, and the
// gitignore and MCP-config hints the generator consults.

var (
	builtinSpecsOnce sync.Once
	builtinSpecs     []*ProviderSpec
)

// loadBuiltinSpecs parses every embedded spec once. An embedded spec that does
// not parse is a build-time bug, so failures panic like providers/init.go.
func loadBuiltinSpecs() []*ProviderSpec {
	builtinSpecsOnce.Do(func() {
		names, err := BuiltinNames()
		if err != nil {
			panic("providers: enumerate builtin specs: " + err.Error())
		}
		sort.Strings(names)
		for _, name := range names {
			gen, err := LoadBuiltin(name)
			if err != nil {
				panic("providers: load builtin " + name + ": " + err.Error())
			}
			builtinSpecs = append(builtinSpecs, gen.Spec)
		}
	})
	return builtinSpecs
}

// sharedDirs are directories users and other tools keep hand-authored files in.
// A generated file in one is ignored by its exact path; the directory (or a
// direct child such as .config/<tool>/) never becomes an ignore pattern, or
// every tracked file beside the generated one would be skipped by git.
// ".agents" is not listed: its generated content lives in owned subdirectories
// (.agents/skills/), which the generator narrows to.
var sharedDirs = []string{".vscode", ".idea", ".zed", ".config", ".github", ".gitlab", ".husky"}

// IsSharedDir reports whether name (one path segment) is a shared directory.
func IsSharedDir(name string) bool { return slices.Contains(sharedDirs, name) }

// GitignoreHints returns what the embedded specs say about generated paths: the
// root instruction files (CLAUDE.md, AGENTS.md, ...) and the dot-directories a
// preset writes into (".claude/", ".agents/", ...), sorted and de-duplicated.
// Directories are the first path segment, with a trailing slash; shared
// directories (see IsSharedDir) are left out, the generator ignores files in
// those by exact path.
func GitignoreHints() (rootFiles []string, dirs []string) {
	files := map[string]bool{}
	dirSet := map[string]bool{}
	for _, spec := range loadBuiltinSpecs() {
		if spec.Root != nil && spec.Root.File != "" && !strings.Contains(spec.Root.File, "/") {
			files[spec.Root.File] = true
		}
		dirPaths, filePaths := specPaths(spec)
		for _, p := range dirPaths {
			if dir := dotDir(p, true); dir != "" {
				dirSet[dir] = true
			}
		}
		for _, p := range filePaths {
			if dir := dotDir(p, false); dir != "" {
				dirSet[dir] = true
			}
		}
	}
	return sortedKeys(files), sortedKeys(dirSet)
}

// IsMCPSidecarKind reports whether a sidecar of this kind holds MCP server
// configuration: the generic "mcp" kind, or one of the legacy tool-specific kinds
// that carry an mcpServers member. It is an explicit allowlist; neither a kind
// name nor a file name that merely contains "mcp" qualifies.
func IsMCPSidecarKind(kind string) bool {
	switch kind {
	case SidecarMCP, SidecarClaudeSettingsJSON, SidecarMCPJSON, SidecarAmpSettingsJSON, SidecarPiMCPJSON:
		return true
	}
	return false
}

// MCPConfigPaths returns the sidecar paths the embedded specs declare as MCP
// configuration (see IsMCPSidecarKind). Paths are repo-relative.
func MCPConfigPaths() []string {
	set := map[string]bool{}
	for _, spec := range loadBuiltinSpecs() {
		for _, sc := range spec.Sidecars {
			if sc != nil && sc.Path != "" && IsMCPSidecarKind(sc.Kind) {
				set[path.Clean(sc.Path)] = true
			}
		}
	}
	return sortedKeys(set)
}

// SidecarPaths returns the project paths of every sidecar the embedded specs
// write, slash-separated and sorted. They are single files, so the generator
// ignores them by exact path instead of through their directory.
func SidecarPaths() []string {
	set := map[string]bool{}
	for _, spec := range loadBuiltinSpecs() {
		for _, sc := range spec.Sidecars {
			if sc != nil && sc.Path != "" {
				set[path.Clean(sc.Path)] = true
			}
		}
	}
	return sortedKeys(set)
}

// specPaths lists every path a spec says it writes other than the root file;
// dirs names directories, files names single files (sidecars).
func specPaths(spec *ProviderSpec) (dirs, files []string) {
	dirs = append(dirs, spec.Directories...)
	for _, out := range spec.Outputs {
		if out != nil && out.Dir != "" {
			dirs = append(dirs, out.Dir)
		}
	}
	for _, sc := range spec.Sidecars {
		if sc != nil && sc.Path != "" {
			files = append(files, sc.Path)
		}
	}
	return dirs, files
}

// dotDir is the leading ".name/" segment of p, or "" when p starts with a
// non-dot segment or a shared directory. A bare p is a directory only when isDir.
func dotDir(p string, isDir bool) string {
	p = strings.TrimPrefix(path.Clean(p), "./")
	first, _, found := strings.Cut(p, "/")
	if !found && !isDir {
		return ""
	}
	if !strings.HasPrefix(first, ".") || IsSharedDir(first) || first == "." || first == ".." {
		return ""
	}
	for _, deep := range nestedOwnedDirs {
		if strings.HasPrefix(p+"/", deep) {
			return deep
		}
	}
	return first + "/"
}

// nestedOwnedDirs are directories below a dot-directory that ai-rulez owns while
// the dot-directory itself holds the user's own files (.takt/ keeps config.yaml
// and workflows beside the generated facets), so they stand in for the leading
// segment as the directory to ignore.
var nestedOwnedDirs = []string{".takt/facets/"}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

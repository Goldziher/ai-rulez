package plugin

import (
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/samber/oops"
)

// includeOutputs resolves the plugin's [plugin] include globs against its content
// root and returns the matched files as verbatim outputs rooted at baseDir,
// keeping each file's relative path. The Agent Plugins specification lets a
// package carry arbitrary files beside plugin.json, mcp.json and skills/ (§4.2),
// and a stdio `command` may be a `./`-relative path into them (§7.2.1); this is
// what makes such a command resolve. Empty Include yields nothing.
func includeOutputs(m *Manifest, baseDir string) ([]config.OutputFile, error) {
	if len(m.Include) == 0 {
		return nil, nil
	}
	root := pluginContentRoot(m)
	matches, err := matchIncludes(root, m.Include)
	if err != nil {
		return nil, err
	}
	outputs := make([]config.OutputFile, 0, len(matches))
	for _, rel := range matches {
		out, err := passthroughFile(root, rel, filepath.Join(baseDir, filepath.FromSlash(rel)))
		if err != nil {
			return nil, err
		}
		out.PluginInclude = true
		outputs = append(outputs, out)
	}
	return outputs, nil
}

// pluginContentRoot is the directory the include globs resolve against:
// [plugin] content_root when set, otherwise the config directory that holds the
// governance content tree. It mirrors how evalsDir resolves the project-level
// eval tree.
func pluginContentRoot(m *Manifest) string {
	if m.Config == nil {
		return m.SourceDir
	}
	return m.Config.Plugin.PluginContentRootDir(m.Config, m.SourceDir)
}

// matchIncludes walks root and returns the root-relative slash paths of the
// regular files matching any include glob, sorted. Paths that would collide with
// the package's own components are skipped so a broad glob cannot overwrite
// plugin.json, mcp.json or a skill.
func matchIncludes(root string, patterns []string) ([]string, error) {
	var matches []string
	err := filepath.WalkDir(root, func(p string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return oops.With("path", p).Wrapf(walkErr, "walk plugin content root")
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return oops.With("path", p).Wrapf(err, "resolve include path")
		}
		rel = filepath.ToSlash(rel)
		if includeReserved(rel) {
			return nil
		}
		for _, pattern := range patterns {
			if matchInclude(pattern, rel) {
				matches = append(matches, rel)
				return nil
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.Sort(matches)
	return slices.Compact(matches), nil
}

// includeReserved reports whether rel is a package component an include must not
// clobber: the manifest, the MCP document, a skill, or the provenance sidecar.
func includeReserved(rel string) bool {
	switch rel {
	case "plugin.json", "mcp.json", provenanceFileName:
		return true
	}
	first, _, _ := strings.Cut(rel, "/")
	return first == "skills"
}

// matchInclude reports whether a content-root-relative slash path matches one
// include glob. `**` crosses directories, `*` and `?` do not, and a trailing `/`
// matches everything below the directory.
func matchInclude(pattern, rel string) bool {
	pattern = strings.TrimPrefix(strings.TrimSpace(pattern), "./")
	if strings.HasSuffix(pattern, "/") {
		pattern += "**"
	}
	if pattern == "" {
		return false
	}
	return matchIncludeSegments(strings.Split(pattern, "/"), strings.Split(rel, "/"))
}

func matchIncludeSegments(pattern, segments []string) bool {
	for len(pattern) > 0 {
		if pattern[0] == "**" {
			if len(pattern) == 1 {
				return true
			}
			for i := 0; i <= len(segments); i++ {
				if matchIncludeSegments(pattern[1:], segments[i:]) {
					return true
				}
			}
			return false
		}
		if len(segments) == 0 {
			return false
		}
		ok, err := path.Match(pattern[0], segments[0])
		if err != nil || !ok {
			return false
		}
		pattern, segments = pattern[1:], segments[1:]
	}
	return len(segments) == 0
}

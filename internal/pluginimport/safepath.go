// Package pluginimport reads the plugin marketplaces and plugins other
// harnesses install (Claude Code, Codex, Cursor, Gemini extensions and Agent
// Plugins) and turns them into ai-rulez content in memory: skills, agents,
// commands and rules as content files, hooks as hook groups and MCP servers.
//
// It never touches the network: the caller fetches a marketplace or plugin
// repository and hands the directory over. Everything it reads from a directory
// is untrusted, so paths are confined to the plugin, symlinks are never followed
// and sizes are bounded.
package pluginimport

import (
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/samber/oops"
)

// Bounds on what one plugin or marketplace may make ai-rulez read.
const (
	// MaxManifestBytes bounds plugin.json, gemini-extension.json and the other small JSON documents.
	MaxManifestBytes = 1 << 20
	// MaxIndexBytes bounds a marketplace.json.
	MaxIndexBytes = 4 << 20
	// MaxContentBytes bounds one markdown file of a plugin.
	MaxContentBytes = 1 << 20
	// MaxFilesPerKind bounds how many agents, commands or rules one plugin delivers.
	MaxFilesPerKind = 500
	// MaxSkills bounds how many skills one plugin delivers.
	MaxSkills = 500
	// MaxIndexPlugins bounds the entries of one marketplace.
	MaxIndexPlugins = 5000
	// maxWalkDepth bounds the directory depth walked below a plugin directory.
	maxWalkDepth = 8
)

var namePattern = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._@-]{0,127}$`) })

// ValidName reports whether name can be a plugin, skill, agent or command name.
func ValidName(name string) bool { return namePattern().MatchString(name) }

// CleanRel validates a path a plugin or marketplace names and returns it
// cleaned, relative and slash-separated. It refuses an absolute path, a drive
// path, a backslash, a control character and any path that leaves the directory
// it is relative to.
func CleanRel(raw string) (string, error) {
	p := strings.TrimSpace(raw)
	switch {
	case p == "":
		return "", oops.Errorf("empty path")
	case strings.ContainsAny(p, "\\\x00") || hasControl(p):
		return "", oops.Errorf("path %q has a backslash or a control character", raw)
	case strings.HasPrefix(p, "/") || strings.HasPrefix(p, "~") || (len(p) >= 2 && p[1] == ':'):
		return "", oops.Errorf("path %q must be relative to the directory that names it", raw)
	}
	clean := path.Clean(p)
	if clean != strings.TrimSpace(clean) {
		return "", oops.Errorf("path %q has leading or trailing whitespace", raw)
	}
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", oops.Errorf("path %q leaves the directory that names it", raw)
	}
	return clean, nil
}

func hasControl(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

// SafeJoin joins a validated relative path to root and checks that no component
// of it below root is a symlink, so the result cannot lead out of root.
func SafeJoin(root, rel string) (string, error) {
	clean, err := CleanRel(rel)
	if err != nil {
		return "", err
	}
	if clean == "." {
		return root, nil
	}
	cur := root
	for _, part := range strings.Split(clean, "/") {
		cur = filepath.Join(cur, part)
		info, statErr := os.Lstat(cur)
		if statErr != nil {
			if os.IsNotExist(statErr) {
				continue
			}
			return "", oops.With("path", cur).Wrapf(statErr, "read %s", rel)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", oops.With("path", cur).Errorf("%s goes through a symlink, and symlinks are not followed", rel)
		}
	}
	return cur, nil
}

// readRegular reads a regular, non-symlink file of at most limit bytes.
func readRegular(file string, limit int64) ([]byte, error) {
	info, err := os.Lstat(file)
	if err != nil {
		return nil, oops.With("path", file).Wrap(err)
	}
	if !info.Mode().IsRegular() {
		return nil, oops.With("path", file).Errorf("%s is a symlink or not a regular file", file)
	}
	if info.Size() > limit {
		return nil, oops.With("path", file).Errorf("%s is larger than %d bytes", file, limit)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, oops.With("path", file).Wrap(err)
	}
	return data, nil
}

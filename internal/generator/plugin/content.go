package plugin

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/samber/oops"
)

// contentLayout describes where a runtime bundles its skills/commands/agents,
// relative to the plugin's content root for that runtime.
type contentLayout struct {
	// Root is the runtime's content root relative to baseDir. Empty means the
	// project root (used by Claude/Kimi, which share top-level skills/ etc.).
	Root string
	// Skills/Commands/Agents toggle which content kinds this runtime bundles.
	Skills   bool
	Commands bool
	Agents   bool
}

// bundleContent emits verbatim copies of the plugin's skills/commands/agents
// into the runtime's content directories. Content files are passed through byte
// for byte from their source path (frontmatter + body), never re-rendered, so a
// bundled SKILL.md is identical to the authored one. Builtin content (loaded
// from embedded sources, path prefixed with "builtin://") is skipped.
func bundleContent(m *Manifest, baseDir string, layout contentLayout) ([]config.OutputFile, error) {
	var outputs []config.OutputFile
	root := filepath.Join(baseDir, layout.Root)

	if layout.Skills {
		for i := range m.Skills {
			out, err := passthroughContent(&m.Skills[i], filepath.Join(root, "skills", m.Skills[i].Name, "SKILL.md"))
			if err != nil {
				return nil, err
			}
			outputs = appendIf(outputs, out)
		}
	}
	if layout.Commands {
		for i := range m.Commands {
			out, err := passthroughContent(&m.Commands[i], filepath.Join(root, "commands", m.Commands[i].Name+".md"))
			if err != nil {
				return nil, err
			}
			outputs = appendIf(outputs, out)
		}
	}
	if layout.Agents {
		for i := range m.Agents {
			out, err := passthroughContent(&m.Agents[i], filepath.Join(root, "agents", m.Agents[i].Name+".md"))
			if err != nil {
				return nil, err
			}
			outputs = appendIf(outputs, out)
		}
	}
	return outputs, nil
}

// passthroughContent reads a content file's source bytes verbatim and returns an
// OutputFile targeting dstPath. Returns a zero OutputFile (skipped) for builtin
// content, which has no on-disk source to copy.
func passthroughContent(cf *config.ContentFile, dstPath string) (config.OutputFile, error) {
	if cf.Path == "" || strings.HasPrefix(cf.Path, "builtin://") {
		return config.OutputFile{}, nil
	}
	data, err := os.ReadFile(cf.Path)
	if err != nil {
		return config.OutputFile{}, oops.With("path", cf.Path).Wrapf(err, "read content source")
	}
	return config.OutputFile{Path: dstPath, RawContent: data}, nil
}

// passthroughFile reads srcPath (resolved relative to the source project when
// not absolute) and returns an OutputFile writing its bytes verbatim to dstPath,
// preserving the executable bit for scripts.
func passthroughFile(sourceDir, srcPath, dstPath string) (config.OutputFile, error) {
	abs := srcPath
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(sourceDir, srcPath)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return config.OutputFile{}, oops.With("path", abs).Wrapf(err, "stat passthrough file")
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return config.OutputFile{}, oops.With("path", abs).Wrapf(err, "read passthrough file")
	}
	return config.OutputFile{Path: dstPath, RawContent: data, Mode: info.Mode().Perm()}, nil
}

// appendIf appends out to outputs unless out is the zero value (a skipped file).
func appendIf(outputs []config.OutputFile, out config.OutputFile) []config.OutputFile {
	if out.Path == "" {
		return outputs
	}
	return append(outputs, out)
}

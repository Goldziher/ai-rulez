package plugin

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/samber/oops"
)

const (
	openCodePluginDependency = "^1.17.8"
	openCodeSourcePath       = ".ai-rulez/opencode/index.js"
)

type openCodePackage struct {
	Name         string             `json:"name"`
	Version      string             `json:"version"`
	Description  string             `json:"description,omitempty"`
	Keywords     []string           `json:"keywords,omitempty"`
	Homepage     string             `json:"homepage,omitempty"`
	Repository   openCodeRepository `json:"repository"`
	License      string             `json:"license,omitempty"`
	Type         string             `json:"type"`
	Main         string             `json:"main"`
	Exports      map[string]string  `json:"exports"`
	Files        []string           `json:"files"`
	Dependencies map[string]string  `json:"dependencies"`
}

type openCodeRepository struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

func init() {
	register(config.PluginRuntimeOpenCode, renderOpenCode)
}

func renderOpenCode(m *Manifest, baseDir string) ([]config.OutputFile, error) {
	entrypoint := filepath.Join(".opencode", "plugins", m.Name+".js")
	module, err := openCodeModule(m, filepath.Join(baseDir, entrypoint))
	if err != nil {
		return nil, err
	}

	pkg, err := jsonOutput(filepath.Join(baseDir, "package.json"), openCodePackage{
		Name:         openCodePackageName(m),
		Version:      m.Version,
		Description:  m.Description,
		Keywords:     m.Keywords,
		Homepage:     m.Homepage,
		Repository:   openCodeRepository{Type: "git", URL: m.Repository},
		License:      m.License,
		Type:         "module",
		Main:         filepath.ToSlash(entrypoint),
		Exports:      map[string]string{".": "./" + filepath.ToSlash(entrypoint)},
		Files:        []string{".opencode/", "assets/", "README.md"},
		Dependencies: map[string]string{"@opencode-ai/plugin": openCodePluginDependency},
	})
	if err != nil {
		return nil, err
	}
	return []config.OutputFile{module, pkg}, nil
}

func openCodePackageName(m *Manifest) string {
	parsed, err := url.Parse(m.Repository)
	if err == nil && strings.EqualFold(parsed.Host, "github.com") {
		segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
		if len(segments) >= 2 && segments[0] != "" {
			return "@" + strings.ToLower(segments[0]) + "/opencode-" + m.Name
		}
	}
	return "opencode-" + m.Name
}

func openCodeModule(m *Manifest, outputPath string) (config.OutputFile, error) {
	sourcePath := filepath.Join(m.SourceDir, openCodeSourcePath)
	info, err := os.Stat(sourcePath)
	if err == nil {
		if info.IsDir() {
			return config.OutputFile{}, oops.With("path", sourcePath).Errorf("OpenCode entrypoint must be a file")
		}
		return passthroughFile(m.SourceDir, sourcePath, outputPath)
	}
	if !os.IsNotExist(err) {
		return config.OutputFile{}, oops.With("path", sourcePath).Wrapf(err, "stat OpenCode entrypoint")
	}
	return config.OutputFile{Path: outputPath, RawContent: []byte(openCodeScaffold(m.Name))}, nil
}

func openCodeScaffold(name string) string {
	return fmt.Sprintf(`/**
 * OpenCode adapter for %s.
 *
 * This generated no-op keeps the plugin loadable without inventing runtime behavior.
 * To add OpenCode-specific tools or hooks:
 *
 * 1. Create .ai-rulez/opencode/index.js.
 * 2. Export an OpenCode plugin function from that source file.
 * 3. Run ai-rulez generate --plugin --dry-run.
 * 4. Run ai-rulez generate --plugin.
 *
 * Keep shared skills, commands, agents, and MCP configuration in their normal
 * .ai-rulez sources. Validate all external input and never interpolate untrusted
 * values into shell commands.
 */
const OpenCodePlugin = async () => ({});

export default OpenCodePlugin;
`, name)
}

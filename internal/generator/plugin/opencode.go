package plugin

import (
	_ "embed"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/presets"
	"github.com/Goldziher/ai-rulez/internal/opencodev1"
	"github.com/samber/oops"
)

// openCodeContentHelper registers the bundled skills, commands, and agents.
//
//go:embed opencode_content.js
var openCodeContentHelper []byte

const (
	// openCodePluginDependency targets OpenCode 2's plugin package, which a v1
	// plugin implementation cannot run against (#194).
	openCodePluginDependency = "^2.0.20"
	openCodeSourcePath       = ".ai-rulez/opencode/index.js"
	openCodeHelperName       = "ai-rulez-content.js"
	openCodeBundleName       = "ai-rulez-bundle.json"
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
	hasContent := len(m.Skills)+len(m.Commands)+len(m.Agents)+len(m.MCP) > 0
	module, err := openCodeModule(m, filepath.Join(baseDir, entrypoint), hasContent)
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
		Dependencies: map[string]string{"@opencode/plugin": openCodePluginDependency},
	})
	if err != nil {
		return nil, err
	}
	outputs := []config.OutputFile{module, pkg}

	// Bundle the plugin's skills, commands, and agents into the OpenCode v2
	// discovery directories so the package is self-contained.
	content, err := bundleContent(m, baseDir, contentLayout{
		Root:     ".opencode",
		Skills:   true,
		Commands: true,
		Agents:   true,
	})
	if err != nil {
		return nil, err
	}

	outputs = append(outputs, content...)
	if hasContent {
		bundle, err := jsonOutput(filepath.Join(baseDir, ".opencode", openCodeBundleName), openCodeBundleFor(m))
		if err != nil {
			return nil, err
		}
		outputs = append(outputs, bundle, config.OutputFile{
			Path:       filepath.Join(baseDir, ".opencode", openCodeHelperName),
			RawContent: openCodeContentHelper,
		})
	}
	return outputs, nil
}

// openCodeBundle is the data the content helper registers at runtime: MCP
// servers in OpenCode's native shape and per-agent settings resolved here, so
// model resolution matches the opencode preset.
type openCodeBundle struct {
	MCP    map[string]openCodeMCP    `json:"mcp,omitempty"`
	Agents map[string]map[string]any `json:"agents,omitempty"`
}

// openCodeMCP is an OpenCode v2 MCP server. ${PLUGIN_ROOT} and ${VAR}
// references are left intact; the helper resolves them at runtime so no secret
// or install path is baked into the package.
type openCodeMCP struct {
	Type        string            `json:"type"`
	Command     []string          `json:"command,omitempty"`
	Environment map[string]string `json:"environment,omitempty"`
	URL         string            `json:"url,omitempty"`
}

func openCodeBundleFor(m *Manifest) openCodeBundle {
	var bundle openCodeBundle
	if len(m.MCP) > 0 {
		bundle.MCP = make(map[string]openCodeMCP, len(m.MCP))
		for _, s := range m.MCP {
			switch s.Transport {
			case config.TransportHTTP, config.TransportSSE:
				bundle.MCP[s.Name] = openCodeMCP{Type: "remote", URL: s.URL}
			default:
				bundle.MCP[s.Name] = openCodeMCP{
					Type:        "local",
					Command:     append([]string{s.Command}, s.Args...),
					Environment: s.Env,
				}
			}
		}
	}
	if len(m.Agents) > 0 {
		cfg := m.Config
		if cfg == nil {
			cfg = &config.Config{}
		}
		bundle.Agents = make(map[string]map[string]any, len(m.Agents))
		for _, agent := range m.Agents {
			bundle.Agents[agent.Name] = presets.OpencodeAgentSettings(agent, cfg)
		}
	}
	return bundle
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

func openCodeModule(m *Manifest, outputPath string, hasContent bool) (config.OutputFile, error) {
	sourcePath := filepath.Join(m.SourceDir, openCodeSourcePath)
	info, err := os.Stat(sourcePath)
	if err == nil {
		if info.IsDir() {
			return config.OutputFile{}, oops.With("path", sourcePath).Errorf("OpenCode entrypoint must be a file")
		}
		out, err := passthroughFile(m.SourceDir, sourcePath, outputPath)
		if err != nil {
			return config.OutputFile{}, err
		}
		opencodev1.WarnSource(sourcePath, string(out.RawContent))
		return out, nil
	}
	if !os.IsNotExist(err) {
		return config.OutputFile{}, oops.With("path", sourcePath).Wrapf(err, "stat OpenCode entrypoint")
	}
	return config.OutputFile{Path: outputPath, RawContent: []byte(openCodeScaffold(m.Name, hasContent))}, nil
}

func openCodeScaffold(name string, hasContent bool) string {
	importLine, setupBody := "", "    // Register hooks, transforms, tools, or subscriptions on ctx here.\n"
	if hasContent {
		importLine = fmt.Sprintf("import { registerBundledContent } from \"../%s\"\n\n", openCodeHelperName)
		setupBody = "    await registerBundledContent(ctx)\n"
	}
	return fmt.Sprintf(`/**
 * OpenCode v2 adapter for %s.
 *
 * To add OpenCode-specific tools or hooks:
 *
 * 1. Create .ai-rulez/opencode/index.js.
 * 2. Default-export { id, setup(ctx) } from that source file. OpenCode v2 does not
 *    run v1 plugins (an exported function returning hooks).
 * 3. If the plugin bundles skills, commands, or agents, call
 *    registerBundledContent(ctx) from ../%s in setup.
 * 4. Run ai-rulez generate --plugin --dry-run, then ai-rulez generate --plugin.
 *
 * Keep shared skills, commands, agents, and MCP configuration in their normal
 * .ai-rulez sources. Validate all external input and never interpolate untrusted
 * values into shell commands.
 */
%s/** @type {import("@opencode/plugin").Plugin.Plugin} */
export default {
  id: %q,
  async setup(ctx) {
%s  },
}
`, name, openCodeHelperName, importLine, name, setupBody)
}

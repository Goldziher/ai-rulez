package plugin

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/presets"
	"github.com/Goldziher/ai-rulez/v5/internal/opencodev1"
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
		Files:        openCodePublishedFiles(m),
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
	// Skills and Commands carry the frontmatter fields the helper registers, keyed
	// by directory and file name, so it never has to parse YAML.
	Skills   map[string]openCodeItem `json:"skills,omitempty"`
	Commands map[string]openCodeItem `json:"commands,omitempty"`
}

// openCodeItem is the frontmatter of a bundled skill or command.
type openCodeItem struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

// openCodeMCP is an OpenCode v2 MCP server. ${PLUGIN_ROOT} and ${VAR}
// references are left intact; the helper resolves them at runtime so no secret
// or install path is baked into the package.
type openCodeMCP struct {
	Type        string            `json:"type"`
	Command     []string          `json:"command,omitempty"`
	Environment map[string]string `json:"environment,omitempty"`
	URL         string            `json:"url,omitempty"`
	Disabled    bool              `json:"disabled,omitempty"`
}

func openCodeBundleFor(m *Manifest) openCodeBundle {
	var bundle openCodeBundle
	if len(m.MCP) > 0 {
		bundle.MCP = make(map[string]openCodeMCP, len(m.MCP))
		for _, s := range m.MCP {
			switch s.Transport {
			case config.TransportHTTP, config.TransportSSE:
				bundle.MCP[s.Name] = openCodeMCP{Type: "remote", URL: s.URL, Disabled: s.Disabled}
			default:
				bundle.MCP[s.Name] = openCodeMCP{
					Type:        "local",
					Command:     append([]string{s.Command}, s.Args...),
					Environment: s.Env,
					Disabled:    s.Disabled,
				}
			}
		}
	}
	bundle.Skills = openCodeItems(m.Skills)
	bundle.Commands = openCodeItems(m.Commands)
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

// openCodeItems maps bundled content to its frontmatter name and description.
func openCodeItems(files []config.ContentFile) map[string]openCodeItem {
	if len(files) == 0 {
		return nil
	}
	items := make(map[string]openCodeItem, len(files))
	for _, file := range files {
		item := openCodeItem{}
		if file.Metadata != nil {
			item.Name = file.Metadata.Extra["name"]
			item.Description = file.Metadata.Extra["description"]
		}
		items[file.Name] = item
	}
	return items
}

// pluginRootRef finds ${PLUGIN_ROOT} followed by a path, whichever separator the
// author used (a Windows launcher is written ${PLUGIN_ROOT}\scripts\run.cmd).
var pluginRootRef = regexp.MustCompile(`\$\{PLUGIN_ROOT\}[\\/]([^\s"'/\\]+)([\\/]?)`)

// openCodePublishedFiles returns the package.json "files" list: the generated
// content plus every top-level source path an MCP server references through
// ${PLUGIN_ROOT}, so a launcher such as scripts/run.sh is published with the
// package. A referenced path missing from the source tree is warned about, not
// listed.
func openCodePublishedFiles(m *Manifest) []string {
	files := []string{".opencode/", "assets/", "README.md"}
	seen := map[string]bool{}
	for _, server := range m.MCP {
		refs := append([]string{server.Command, server.URL}, server.Args...)
		for _, value := range server.Env {
			refs = append(refs, value)
		}
		for _, ref := range refs {
			for _, match := range pluginRootRef.FindAllStringSubmatch(ref, -1) {
				top := match[1]
				if top == ".." || top == "." || seen[top] {
					continue
				}
				seen[top] = true
				info, err := os.Stat(filepath.Join(m.SourceDir, top))
				if err != nil {
					m.log().Warn("MCP server references a path missing from the plugin source; it will not be published",
						"server", server.Name, "path", top)
					continue
				}
				entry := top
				if info.IsDir() {
					entry += "/"
				}
				if !slices.Contains(files, entry) {
					files = append(files, entry)
				}
			}
		}
	}
	sort.Strings(files[3:])
	return files
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
		opencodev1.WarnSource(m.Config.Collector(), m.log(), sourcePath, string(out.RawContent))
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
  id: %s,
  async setup(ctx) {
%s  },
}
`, commentSafe(name), openCodeHelperName, importLine, jsString(name), setupBody)
}

// jsString renders value as a JavaScript string literal. Go's %q is not one: it
// emits escapes such as \x00 and \U0001F600 that JavaScript reads differently.
func jsString(value string) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return `""`
	}
	return string(encoded)
}

// commentSafe makes value harmless inside a block comment: it can neither end the
// comment nor start a new line.
func commentSafe(value string) string {
	return strings.NewReplacer("*/", "* /", "\r", " ", "\n", " ").Replace(value)
}

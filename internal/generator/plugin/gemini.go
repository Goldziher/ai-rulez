package plugin

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/samber/oops"
)

func init() {
	register(config.PluginRuntimeGemini, renderGemini)
}

// defaultGeminiContextFile is the context filename Gemini loads when the plugin
// does not override it.
const defaultGeminiContextFile = "GEMINI.md"

// geminiManifest is the shape of gemini-extension.json. Gemini uses inline MCP
// servers (with ${extensionPath}) and an inline hooks block, and points at a
// markdown context file rather than a skills directory.
type geminiManifest struct {
	Name            string                    `json:"name"`
	Version         string                    `json:"version"`
	Description     string                    `json:"description,omitempty"`
	ContextFileName string                    `json:"contextFileName,omitempty"`
	MCPServers      map[string]mcpEntry       `json:"mcpServers,omitempty"`
	Hooks           map[string][]hookGroupDoc `json:"hooks,omitempty"`
}

func renderGemini(m *Manifest, baseDir string) ([]config.OutputFile, error) {
	contextFile := defaultGeminiContextFile
	if m.Gemini != nil && m.Gemini.ContextFileName != "" {
		contextFile = m.Gemini.ContextFileName
	}

	doc := geminiManifest{
		Name:            m.Name,
		Version:         m.Version,
		Description:     m.Description,
		ContextFileName: contextFile,
		MCPServers:      mcpServersFor(m, config.PluginRuntimeGemini),
		Hooks:           hooksBlock(m, config.PluginRuntimeGemini),
	}

	// Gemini inlines the hooks block instead of writing hooks.json, but a
	// script-declared action still renders a command addressing hooks/<basename>.
	// The scripts have to be bundled here too, or that command dangles.
	outputs, err := bundleHookScripts(m, filepath.Join(baseDir, hooksDirName))
	if err != nil {
		return nil, err
	}

	out, err := jsonOutput(filepath.Join(baseDir, "gemini-extension.json"), doc)
	if err != nil {
		return nil, err
	}
	outputs = append(outputs, out)

	if m.Gemini != nil && m.Gemini.Commands {
		commands, err := geminiCommands(m, baseDir)
		if err != nil {
			return nil, err
		}
		outputs = append(outputs, commands...)
	}
	return outputs, nil
}

// geminiCommands converts each bundled command to a Gemini custom command,
// commands/<name>.toml with a required prompt and an optional description
// (https://geminicli.com/docs/cli/custom-commands/). The Claude-style
// $ARGUMENTS placeholder becomes Gemini's {{args}}; the body is otherwise kept.
// Agents and hooks are not bundled: the extension reference links their formats
// elsewhere and ai-rulez does not translate into an unverified one.
func geminiCommands(m *Manifest, baseDir string) ([]config.OutputFile, error) {
	var outputs []config.OutputFile
	for i := range m.Commands {
		cf := &m.Commands[i]
		if cf.Path == "" || strings.HasPrefix(cf.Path, "builtin://") {
			continue
		}
		data, err := os.ReadFile(cf.Path)
		if err != nil {
			return nil, oops.With("path", cf.Path).Wrapf(err, "read command source")
		}
		meta, body := config.ParseFrontmatterPublic(string(data))
		body = strings.ReplaceAll(strings.TrimSpace(body), "$ARGUMENTS", "{{args}}")
		var toml strings.Builder
		if desc := config.SkillDescription(meta); desc != "" {
			toml.WriteString("description = " + tomlBasicString(desc) + "\n")
		}
		toml.WriteString("prompt = \"\"\"\n" + tomlMultiline(body) + "\n\"\"\"\n")
		outputs = append(outputs, config.OutputFile{
			Path:       filepath.Join(baseDir, "commands", cf.Name+".toml"),
			RawContent: []byte(toml.String()),
		})
	}
	return outputs, nil
}

// tomlBasicString renders s as a single-line TOML basic string.
func tomlBasicString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// tomlMultiline escapes s for the inside of a TOML multi-line basic string.
func tomlMultiline(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	return strings.ReplaceAll(s, `"""`, `\"\"\"`)
}

package lint

import (
	"context"
	"os/exec"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func TestPluginManifestAR963(t *testing.T) {
	plugin := func(body string) map[string]string {
		return map[string]string{"plugins/p/.claude-plugin/plugin.json": body}
	}
	market := func(body string) map[string]string {
		return map[string]string{".claude-plugin/marketplace.json": body}
	}
	runRuleCases(t, []ruleCase{
		{name: "valid plugin", files: plugin(`{"name":"my-plugin","version":"1.2.3","description":"d","author":{"name":"me"},"keywords":["a"],"skills":"./skills","commands":["./commands/a.md"]}`), absent: []string{"AR963"}},
		{name: "missing name", files: plugin(`{"version":"1.0.0"}`), want: []string{"AR963:plugin.json:1"}},
		{name: "name with spaces", files: plugin(`{"name":"My Plugin"}`), want: []string{"AR963:plugin.json:1"}},
		{name: "version not semver", files: plugin(`{"name":"p","version":"1.0"}`), want: []string{"AR963:plugin.json:1"}},
		{name: "author as a string", files: plugin(`{"name":"p","author":"me"}`), want: []string{"AR963:plugin.json:1"}},
		{name: "path without ./", files: plugin(`{"name":"p","commands":"commands"}`), want: []string{"AR963:plugin.json:1"}},
		{name: "path escapes", files: plugin(`{"name":"p","skills":"./../x"}`), want: []string{"AR963:plugin.json:1"}},
		{name: "unknown field is a warning with a hint", files: plugin(`{"name":"p","descrption":"x"}`), want: []string{"AR963:plugin.json:1"}, sev: map[string]Severity{"AR963": SeverityWarning}},
		{name: "invalid json", files: plugin(`{"name":`), want: []string{"AR963:plugin.json:1"}},
		{
			name: "unquoted plugin root", files: plugin(`{"name":"p","hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"${CLAUDE_PLUGIN_ROOT}/hooks/go.sh"}]}]}}`),
			want: []string{"AR963:plugin.json:1"},
		},
		{
			name: "quoted plugin root is fine", files: plugin(`{"name":"p","hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"\"${CLAUDE_PLUGIN_ROOT}/hooks/go.sh\" --flag"}]}]}}`),
			absent: []string{"AR963"},
		},
		{
			name:  "unquoted plugin root in hooks.json",
			files: map[string]string{"plugins/p/hooks/hooks.json": `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"sh ${CLAUDE_PLUGIN_ROOT}/a.sh"}]}]}}`},
			want:  []string{"AR963:hooks.json:1"},
		},
		{
			name:   "exec form needs no quotes",
			files:  map[string]string{"plugins/p/hooks/hooks.json": `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"${CLAUDE_PLUGIN_ROOT}/a.sh","args":["x"]}]}]}}`},
			absent: []string{"AR963"},
		},
		{name: "valid marketplace", files: market(`{"name":"acme-tools","owner":{"name":"Acme"},"plugins":[{"name":"a","source":"./plugins/a"},{"name":"b","source":{"source":"github","repo":"o/r"}}]}`), absent: []string{"AR963"}},
		{name: "reserved marketplace name", files: market(`{"name":"claude-code-plugins","owner":{"name":"A"},"plugins":[]}`), want: []string{"AR963:marketplace.json:1"}},
		{name: "marketplace without owner", files: market(`{"name":"acme","plugins":[]}`), want: []string{"AR963:marketplace.json:1"}},
		{name: "plugin source without ./", files: market(`{"name":"acme","owner":{"name":"A"},"plugins":[{"name":"a","source":"plugins/a"}]}`), want: []string{"AR963:marketplace.json:1"}},
		{name: "duplicate plugin", files: market(`{"name":"acme","owner":{"name":"A"},"plugins":[{"name":"a","source":"./a"},{"name":"a","source":"./b"}]}`), want: []string{"AR963:marketplace.json:1"}},
		{name: "unknown source kind", files: market(`{"name":"acme","owner":{"name":"A"},"plugins":[{"name":"a","source":{"source":"nope"}}]}`), want: []string{"AR963:marketplace.json:1"}},
		{name: "off", files: plugin(`{}`), config: "\n[lint.severity]\nAR963 = \"off\"\n", absent: []string{"AR963"}},
	})
}

func TestPluginManifestDelegatesToClaudeWhenAvailable(t *testing.T) {
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("claude binary not available")
	}
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		".ai-rulez/config.toml":                baseConfig,
		"plugins/p/.claude-plugin/plugin.json": `{"name":"p","hooks":"./missing-hooks.json"}`,
	})
	gitAdd(t, root)
	cfg, err := config.LoadConfig(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	tree, err := LoadTree(root)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := RunWith(cfg, tree, Options{External: true})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range rep.Findings {
		if f.Code == CodePluginManifest && len(f.Message) > 24 && f.Message[:24] == "claude plugin validate: " {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a finding from claude plugin validate\n%s", dump(rep.Findings))
	}
}

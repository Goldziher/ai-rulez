package importer

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"

	"github.com/kaptinlin/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// rulesyncTargets is the tool-target list of rulesync (src/types/tool-target-tuples.ts of
// dyoshikawa/rulesync): every name must be either mapped to a preset or reported as unsupported.
var rulesyncTargets = []string{
	"agentsmd", "agentsskills", "aiassistant", "amp", "antigravity-cli", "antigravity-ide", "antigravity-plugin",
	"augmentcode-legacy", "augmentcode-plugin", "augmentcode", "bob", "claudecode-legacy", "claudecode-plugin",
	"claudecode", "cline", "codebuddy", "codebuff", "codewhale", "codexcli", "commandcode", "continue", "copilot",
	"copilotcli", "cortexcode", "crush", "cursor", "deepagents", "devin-plugin", "devin", "dsh", "factorydroid",
	"gitlabduo", "goose", "grokcli", "hermesagent", "junie", "kilo", "kimi-code-plugin", "kimi-code", "kiro-cli",
	"kiro-ide", "kiro", "lettacode", "mimocode", "musecode", "omp", "openclaw", "opencode", "pi", "pool", "qoder",
	"qwencode", "reasonix", "replit", "roo", "rovodev", "tabnine", "takt", "trae", "vibe-plugin", "vibe", "warp",
	"warpcli", "zcode-plugin", "zcode", "zed", "zoocode",
}

func rulesyncPlan(t *testing.T, files map[string]string) *Plan {
	t.Helper()
	return planOf(t, rulesyncImporter{}, mapFS(files), Options{})
}

func TestRulesyncTargets_CoverEveryRulesyncTool(t *testing.T) {
	presets := map[string]bool{}
	for _, n := range config.IndividualPresetNames() {
		presets[n] = true
	}
	for _, name := range rulesyncTargets {
		preset, mapped := rulesyncPresets[name]
		_, unsupported := rulesyncUnsupported[name]
		assert.NotEqual(t, mapped, unsupported, "%s must be mapped or unsupported, not both or neither", name)
		if mapped {
			assert.True(t, presets[preset], "%s maps to %q, which is not an ai-rulez preset", name, preset)
		}
	}
	for name, preset := range rulesyncPresets {
		assert.True(t, presets[preset], "%s maps to %q, which is not an ai-rulez preset", name, preset)
	}
}

func TestMapItemTargets(t *testing.T) {
	tests := []struct {
		name         string
		raw          []string
		want         []string
		wantStatus   Status
		wantNoFinder bool
	}{
		{name: "absent means everywhere", raw: nil, want: nil, wantNoFinder: true},
		{name: "wildcard means everywhere", raw: []string{"*"}, want: nil, wantNoFinder: true},
		{name: "wildcard wins over names", raw: []string{"cursor", "*"}, want: nil, wantNoFinder: true},
		{name: "names map to presets", raw: []string{"claudecode", "cursor", "codexcli"}, want: []string{"claude", "codex", "cursor"}, wantNoFinder: true},
		{name: "agentsmd selects the shared root file", raw: []string{"agentsmd"}, want: []string{"AGENTS.md"}, wantNoFinder: true},
		{name: "partly unmapped is approximated", raw: []string{"cursor", "claudecode-plugin"}, want: []string{"cursor"}, wantStatus: StatusApproximated},
		{name: "fully unmapped keeps the names and needs action", raw: []string{"tabnine"}, want: []string{"tabnine"}, wantStatus: StatusNeedsAction},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			p := &Plan{}

			// Act
			got := mapItemTargets(p, "x.md", tt.raw)

			// Assert
			assert.Equal(t, tt.want, got)
			if tt.wantNoFinder {
				assert.Empty(t, p.Findings)
				return
			}
			require.Len(t, p.Findings, 1)
			assert.Equal(t, tt.wantStatus, p.Findings[0].Status)
		})
	}
}

func TestRulesyncPlan_RuleFrontmatter(t *testing.T) {
	tests := []struct {
		name        string
		file        string
		wantItem    string
		wantFM      []string // lines the frontmatter must contain
		notWantFM   []string
		wantFinding []struct {
			status Status
			field  string
		}
	}{
		{
			name:     "plain rule",
			file:     "---\ntargets: [\"*\"]\ndescription: Style\n---\nUse tabs.\n",
			wantItem: "rules/style.md",
			wantFM:   []string{"description: Style"},
		},
		{
			name:     "globs scope the rule",
			file:     "---\nglobs: [\"src/**/*.go\"]\n---\nWrap errors.\n",
			wantItem: "rules/style.md",
			wantFM:   []string{"activation: glob", "- src/**/*.go"},
		},
		{
			name:      "catch-all globs mean always",
			file:      "---\nglobs: [\"**/*\"]\ndescription: Everywhere\n---\nBe kind.\n",
			wantItem:  "rules/style.md",
			notWantFM: []string{"globs", "activation"},
		},
		{
			name:     "root rule becomes context",
			file:     "---\nroot: true\ndescription: Overview\n---\nHello.\n",
			wantItem: "context/style.md",
			wantFM:   []string{"description: Overview"},
			wantFinding: []struct {
				status Status
				field  string
			}{{StatusApproximated, "root"}},
		},
		{
			name:     "devin trigger sets activation",
			file:     "---\ndescription: Tests\ndevin:\n  trigger: manual\n---\nRun tests.\n",
			wantItem: "rules/style.md",
			wantFM:   []string{"activation: manual"},
			wantFinding: []struct {
				status Status
				field  string
			}{{StatusApproximated, "devin.trigger"}},
		},
		{
			name:     "kiro inclusion with file match",
			file:     "---\nkiro:\n  inclusion: fileMatch\n  fileMatchPattern: [\"web/**\"]\n---\nUse React.\n",
			wantItem: "rules/style.md",
			wantFM:   []string{"activation: glob", "- web/**"},
		},
		{
			name:     "claudecode paths fill in missing globs",
			file:     "---\nclaudecode:\n  paths: [\"api/**\"]\n---\nValidate input.\n",
			wantItem: "rules/style.md",
			wantFM:   []string{"- api/**"},
			wantFinding: []struct {
				status Status
				field  string
			}{{StatusApproximated, "claudecode.paths"}},
		},
		{
			name:     "specific globs win over a section trigger",
			file:     "---\nglobs: [\"a/**\"]\ndevin:\n  trigger: manual\n---\nBody.\n",
			wantItem: "rules/style.md",
			wantFM:   []string{"activation: glob"},
			wantFinding: []struct {
				status Status
				field  string
			}{{StatusDropped, "devin"}},
		},
		{
			name:     "unknown scalar key is dropped with a finding",
			file:     "---\nmystery: 1\n---\nBody.\n",
			wantItem: "rules/style.md",
			wantFinding: []struct {
				status Status
				field  string
			}{{StatusDropped, "mystery"}},
		},
		{
			name:     "targets are mapped",
			file:     "---\ntargets: [\"claudecode\", \"cursor\"]\n---\nBody.\n",
			wantItem: "rules/style.md",
			wantFM:   []string{"- claude", "- cursor"},
		},
		{
			name:     "no frontmatter",
			file:     "Just text.\n",
			wantItem: "rules/style.md",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			files := map[string]string{".rulesync/rules/style.md": tt.file}

			// Act
			p := rulesyncPlan(t, files)

			// Assert
			require.Equal(t, []string{tt.wantItem}, itemRels(p))
			text := string(p.Items[0].Main)
			for _, w := range tt.wantFM {
				assert.Contains(t, text, w)
			}
			for _, w := range tt.notWantFM {
				assert.NotContains(t, text, w)
			}
			for _, f := range tt.wantFinding {
				assert.NotNil(t, findingFor(p, f.status, ".rulesync/rules/style.md", f.field), "want %s %s in %+v", f.status, f.field, p.Findings)
			}
		})
	}
}

func TestRulesyncPlan_LocalRootIsNotImported(t *testing.T) {
	// Arrange
	files := map[string]string{".rulesync/rules/me.md": "---\nlocalRoot: true\n---\nPersonal.\n"}

	// Act
	p := rulesyncPlan(t, files)

	// Assert
	assert.Empty(t, p.Items)
	assert.NotNil(t, findingFor(p, StatusNeedsAction, ".rulesync/rules/me.md", "localRoot"))
}

// TestRulesyncPlan_NoFrontmatterKeyIsSilent feeds every frontmatter key and tool section
// rulesync documents and requires that each one is either carried into the output or
// reported: nothing may disappear without a finding.
func TestRulesyncPlan_NoFrontmatterKeyIsSilent(t *testing.T) {
	ruleSections := []string{"agentsmd", "claudecode", "codebuddy", "continue", "cursor", "trae", "copilot", "antigravity",
		"aiassistant", "devin", "qoder", "augmentcode", "kiro", "pi", "roo", "takt", "factorydroid"}
	tests := []struct {
		name     string
		path     string
		scalars  []string
		sections []string
	}{
		{"rule", ".rulesync/rules/x.md", []string{"root", "localRoot", "targets", "description", "globs"}, ruleSections},
		{"command", ".rulesync/commands/x.md", []string{"targets", "description"}, []string{"copilot", "antigravity", "takt", "pi", "codexcli", "roo"}},
		{"subagent", ".rulesync/subagents/x.md", []string{"name", "targets", "description"},
			[]string{"claudecode", "copilot", "copilotcli", "opencode", "kilo", "cursor", "junie", "takt", "roo", "bob", "zoocode", "zcode", "pool", "vibe"}},
		{"check", ".rulesync/checks/x.md", []string{"targets", "description", "severity", "tools"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange: a section carries a key that no mapping knows.
			var fm strings.Builder
			fm.WriteString("---\n")
			for _, s := range tt.scalars {
				switch s {
				case "root", "localRoot":
					fm.WriteString(s + ": false\n")
				case "targets", "globs", "tools":
					fm.WriteString(s + ": [\"x\"]\n")
				default:
					fm.WriteString(s + ": value\n")
				}
			}
			for _, s := range tt.sections {
				fm.WriteString(s + ":\n  zz-unknown-key: 1\n")
			}
			fm.WriteString("zz-top-level: 1\n---\nBody text.\n")

			// Act
			p := rulesyncPlan(t, map[string]string{tt.path: fm.String()})

			// Assert
			for _, s := range tt.sections {
				f := findingFor(p, StatusDropped, tt.path, s)
				if assert.NotNil(t, f, "section %s was dropped silently", s) {
					assert.Contains(t, f.Reason, "zz-unknown-key")
				}
			}
			assert.NotNil(t, findingFor(p, StatusDropped, tt.path, "zz-top-level"))
		})
	}
}

func TestRulesyncPlan_Commands(t *testing.T) {
	// Arrange
	files := map[string]string{
		".rulesync/commands/review.md":     "---\ndescription: Review\ntargets: [\"claudecode\"]\ncopilot:\n  agent: agent\n---\nReview $ARGUMENTS.\n",
		".rulesync/commands/git/commit.md": "Commit.\n",
		".rulesync/commands/empty.md":      "---\ndescription: x\n---\n\n",
	}

	// Act
	p := rulesyncPlan(t, files)

	// Assert
	assert.Equal(t, []string{"commands/git-commit.md", "commands/review.md"}, itemRels(p))
	assert.Contains(t, string(p.Items[1].Main), "Review $ARGUMENTS.")
	assert.NotNil(t, findingFor(p, StatusDropped, ".rulesync/commands/review.md", "copilot"))
	assert.NotNil(t, findingFor(p, StatusApproximated, ".rulesync/commands/git/commit.md", "name"))
	assert.NotNil(t, findingFor(p, StatusDropped, ".rulesync/commands/empty.md", ""))
}

func TestRulesyncPlan_Subagents(t *testing.T) {
	tests := []struct {
		name    string
		file    string
		want    []string
		notWant []string
	}{
		{
			name:    "claudecode keys are lifted",
			file:    "---\nname: planner\ndescription: Plans\nclaudecode:\n  model: opus\n  tools: Read, Grep\n  skills: [lint]\n  effort: high\n---\nPlan.\n",
			want:    []string{"name: planner", "model: opus", "- Read", "- Grep", "skills:", "effort: high"},
			notWant: []string{"claudecode"},
		},
		{
			name:    "inherit model is the default and is not written",
			file:    "---\nname: a\nclaudecode:\n  model: inherit\n---\nDo.\n",
			notWant: []string{"model"},
		},
		{
			name: "name falls back to the file name",
			file: "---\ndescription: x\n---\nDo.\n",
			want: []string{"name: planner"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			p := rulesyncPlan(t, map[string]string{".rulesync/subagents/planner.md": tt.file})

			// Assert
			require.Equal(t, []string{"agents/planner.md"}, itemRels(p))
			text := string(p.Items[0].Main)
			for _, w := range tt.want {
				assert.Contains(t, text, w)
			}
			for _, w := range tt.notWant {
				assert.NotContains(t, text, w)
			}
		})
	}
}

func TestRulesyncPlan_Skills(t *testing.T) {
	// Arrange
	files := map[string]string{
		".rulesync/skills/lint/SKILL.md":            "---\nname: other\ndescription: Lint\nlicense: MIT\nclaudecode:\n  allowed-tools: Bash\n  context: fork\n---\nRun lint.\n",
		".rulesync/skills/lint/scripts/run.sh":      "echo hi\n",
		".rulesync/skills/.curated/shared/SKILL.md": "---\nname: shared\ndescription: S\n---\nShared.\n",
		".rulesync/skills/nodoc/README.md":          "no skill file\n",
	}

	// Act
	p := rulesyncPlan(t, files)

	// Assert
	assert.Equal(t, []string{"skills/lint/SKILL.md", "skills/shared/SKILL.md"}, itemRels(p))
	lint := p.Items[0]
	assert.Contains(t, string(lint.Main), "name: lint", "name follows the directory")
	assert.Contains(t, string(lint.Main), "allowed-tools: Bash")
	require.Len(t, lint.Resources, 1)
	assert.Equal(t, "scripts/run.sh", lint.Resources[0].Path)
	assert.NotNil(t, findingFor(p, StatusApproximated, ".rulesync/skills/lint/SKILL.md", "name"))
	assert.NotNil(t, findingFor(p, StatusDropped, ".rulesync/skills/lint/SKILL.md", "claudecode"))
	assert.NotNil(t, findingFor(p, StatusDropped, ".rulesync/skills/nodoc", ""))
}

func TestRulesyncPlan_Checks(t *testing.T) {
	// Arrange
	files := map[string]string{
		".rulesync/checks/sec.md": "---\ndescription: Sec\nseverity: HIGH\ntools: [Read]\ntargets: [cursor]\n---\nLook.\n",
		".rulesync/checks/bad.md": "---\nseverity: urgent\n---\nLook.\n",
	}

	// Act
	p := rulesyncPlan(t, files)

	// Assert
	assert.Equal(t, []string{"checks/bad.md", "checks/sec.md"}, itemRels(p))
	assert.Contains(t, string(p.Items[1].Main), "severity: high")
	assert.NotContains(t, string(p.Items[0].Main), "severity")
	assert.NotNil(t, findingFor(p, StatusDropped, ".rulesync/checks/bad.md", "severity"))
}

func TestRulesyncPlan_MCP(t *testing.T) {
	// Arrange
	files := map[string]string{".rulesync/mcp.jsonc": `{
  // comment
  "$schema": "x",
  "mcpServers": {
    "$schema": "y",
    "a": {"type": "local", "command": "npx", "args": ["-y", "a"], "env": {"TOKEN": "${TOKEN}"}, "enabledTools": ["t"]},
    "b": {"type": "streamable-http", "url": "https://example.com/mcp", "headers": {"Authorization": "Bearer ${TOKEN}"}, "targets": ["cursor"]},
    "c": {"type": "ws", "url": "wss://example.com"},
    "d": {"command": "x", "enabled": false,},
  },
  "claudecode": {"mcpServers": {"only": {"command": "y"}, "a": null}},
  "extra": 1,
}`}

	// Act
	p := rulesyncPlan(t, files)

	// Assert
	var names []string
	for _, s := range p.MCPServers {
		names = append(names, s.Name+":"+s.GetTransport())
	}
	assert.Equal(t, []string{"a:stdio", "b:http", "d:stdio"}, names)
	assert.False(t, p.MCPServers[2].IsEnabled())
	assert.Equal(t, "Bearer ${TOKEN}", p.MCPServers[1].Headers["Authorization"])
	src := ".rulesync/mcp.jsonc"
	assert.NotNil(t, findingFor(p, StatusDropped, src, "mcpServers.a.enabledTools"))
	assert.NotNil(t, findingFor(p, StatusApproximated, src, "mcpServers.b.targets"))
	assert.NotNil(t, findingFor(p, StatusUnsupported, src, "mcpServers.c.type"))
	assert.NotNil(t, findingFor(p, StatusDropped, src, "claudecode.mcpServers"))
	assert.NotNil(t, findingFor(p, StatusDropped, src, "extra"))
}

func TestRulesyncPlan_MCPPrecedenceAndInvalid(t *testing.T) {
	// Arrange
	files := map[string]string{
		".rulesync/mcp.jsonc":  `{"mcpServers":{"a":{"command":"a"}}}`,
		".rulesync/mcp.json":   `{"mcpServers":{"legacy":{"command":"l"}}}`,
		".rulesync/hooks.json": `not json`,
	}

	// Act
	p := rulesyncPlan(t, files)

	// Assert
	require.Len(t, p.MCPServers, 1)
	assert.Equal(t, "a", p.MCPServers[0].Name)
	assert.NotNil(t, findingFor(p, StatusDropped, ".rulesync/mcp.json", ""))
	assert.NotNil(t, findingFor(p, StatusUnsupported, ".rulesync/hooks.json", ""))
}

func TestRulesyncPlan_HooksAndPermissionsAreImported(t *testing.T) {
	// Arrange
	files := map[string]string{
		".rulesync/hooks.jsonc":       `{"version":1,"hooks":{"sessionStart":[{"command":"x"}]},"claudecode":{"hooks":{"preToolUse":[]}}}`,
		".rulesync/permissions.jsonc": `{"permission":{"bash":{"rm *":"deny"}},"codexcli":{"approval_policy":"on-request"}}`,
	}

	// Act
	p := rulesyncPlan(t, files)

	// Assert
	require.Len(t, p.Hooks, 1)
	assert.Equal(t, "SessionStart", p.Hooks[0].Event)
	assert.Equal(t, []string{"Bash(rm *)"}, p.Permissions.Deny)
	assert.NotNil(t, findingFor(p, StatusDropped, ".rulesync/permissions.jsonc", "codexcli"))
}

func TestRulesyncPlan_Config(t *testing.T) {
	tests := []struct {
		name        string
		cfg         string
		wantPresets []string
		wantFinding []struct {
			status Status
			field  string
		}
	}{
		{
			name:        "array targets",
			cfg:         `{"targets":["claudecode","cursor","tabnine"],"features":["rules"]}`,
			wantPresets: []string{"claude", "cursor"},
			wantFinding: []struct {
				status Status
				field  string
			}{{StatusUnsupported, "targets.tabnine"}, {StatusApproximated, "features"}},
		},
		{
			name:        "wildcard needs action",
			cfg:         `{"targets":["*"]}`,
			wantPresets: nil,
			wantFinding: []struct {
				status Status
				field  string
			}{{StatusNeedsAction, "targets.*"}},
		},
		{
			name: "settings of the generator are reported",
			cfg:  `{"delete":true,"language":"ja","global":true,"simulateSkills":true,"mystery":1,"sources":[{"source":"a/b"}]}`,
			wantFinding: []struct {
				status Status
				field  string
			}{
				{StatusDropped, "delete"}, {StatusDropped, "language"}, {StatusDropped, "global"},
				{StatusDropped, "simulateSkills"}, {StatusDropped, "mystery"}, {StatusNeedsAction, "sources.a/b"},
			},
		},
		{
			name: "all features is silent",
			cfg:  `{"features":["*"]}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			p := rulesyncPlan(t, map[string]string{"rulesync.jsonc": tt.cfg, ".rulesync/rules/x.md": "Body.\n"})

			// Assert
			assert.Equal(t, tt.wantPresets, p.Presets)
			for _, f := range tt.wantFinding {
				assert.NotNil(t, findingFor(p, f.status, "rulesync.jsonc", f.field), "want %s %s in %+v", f.status, f.field, p.Findings)
			}
			if len(tt.wantFinding) == 0 {
				assert.Nil(t, findingFor(p, StatusApproximated, "rulesync.jsonc", "features"))
			}
		})
	}
}

func TestRulesyncPlan_InvalidConfigIsAnError(t *testing.T) {
	// Act
	_, err := rulesyncImporter{}.Plan(mapFS(map[string]string{"rulesync.jsonc": "{ not json"}), Options{})

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), CodeInvalid)
}

func TestRulesyncPlan_InputRoots(t *testing.T) {
	// Arrange
	files := map[string]string{
		"rulesync.jsonc":               `{"inputRoots":[".rulesync","overlay/.rulesync","../outside","/abs"]}`,
		".rulesync/rules/a.md":         "Base a.\n",
		".rulesync/rules/b.md":         "Base b.\n",
		"overlay/.rulesync/rules/a.md": "Overlay a.\n",
	}

	// Act
	p := rulesyncPlan(t, files)

	// Assert
	assert.Equal(t, []string{"rules/a.md", "rules/b.md"}, itemRels(p))
	assert.Contains(t, string(p.Items[0].Main), "Overlay a.")
	assert.NotNil(t, findingFor(p, StatusApproximated, "overlay/.rulesync/rules/a.md", "name"))
	assert.NotNil(t, findingFor(p, StatusUnsupported, "rulesync.jsonc", "inputRoots"))
}

func TestRulesyncPlan_IgnoreAndLocalFiles(t *testing.T) {
	// Arrange
	files := map[string]string{
		".rulesync/rules/x.md":   "Body.\n",
		".rulesync/.aiignore":    "tmp/\n# c\n\nsecrets/\n",
		".rulesyncignore":        "old/\n",
		"rulesync.local.jsonc":   `{}`,
		"rulesync-npm.lock.json": `{}`,
		".rulesync/notes.txt":    "x",
	}

	// Act
	p := rulesyncPlan(t, files)

	// Assert
	ignore := findingFor(p, StatusDropped, ".rulesync/.aiignore", "")
	require.NotNil(t, ignore)
	assert.Contains(t, ignore.Reason, "2 ignore pattern")
	assert.NotNil(t, findingFor(p, StatusDropped, ".rulesyncignore", ""))
	assert.NotNil(t, findingFor(p, StatusNeedsAction, "rulesync.local.jsonc", ""))
	assert.NotNil(t, findingFor(p, StatusNeedsAction, "rulesync-npm.lock.json", ""))
	assert.NotNil(t, findingFor(p, StatusDropped, ".rulesync/notes.txt", ""))
}

func TestRulesyncPlan_SymlinksAreNotFollowed(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.md")
	require.NoError(t, os.WriteFile(outside, []byte("outside\n"), 0o644))
	writeTree(t, dir, map[string]string{".rulesync/rules/ok.md": "Fine.\n"})
	testutil.SymlinkOrSkip(t, outside, filepath.Join(dir, ".rulesync", "rules", "link.md"))

	// Act
	p, err := rulesyncImporter{}.Plan(os.DirFS(dir), Options{})

	// Assert
	require.NoError(t, err)
	p.Finalize()
	assert.Equal(t, []string{"rules/ok.md"}, itemRels(p))
	assert.NotNil(t, findingFor(p, StatusDropped, ".rulesync/rules/link.md", ""))
}

func TestRulesyncDetect(t *testing.T) {
	// Arrange
	files := map[string]string{
		"rulesync.jsonc":         "{}",
		".rulesync/rules/a.md":   "x",
		".rulesync/mcp.jsonc":    "{}",
		".rulesync/unrelated.md": "x",
	}

	// Act
	got := rulesyncImporter{}.Detect(mapFS(files))

	// Assert
	assert.Equal(t, []string{".rulesync/mcp.jsonc", ".rulesync/rules", "rulesync.jsonc"}, got)
	assert.Empty(t, rulesyncImporter{}.Detect(mapFS(map[string]string{"CLAUDE.md": "x"})))
}

func rulesyncProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	copyDir(t, filepath.Join("testdata", "convert", "rulesync", "in"), dir)
	return dir
}

func TestConvert_RulesyncAutoPrefersRulesyncOverGeneratedFiles(t *testing.T) {
	// Arrange
	dir := rulesyncProject(t)

	// Act
	report, err := Convert(context.Background(), ConvertOptions{Source: dir})

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "rulesync", report.Importer)
	assert.NotNil(t, findingForReport(report, StatusDropped, "(native files)"))
}

func TestConvert_RulesyncExplicitBothKeepsNative(t *testing.T) {
	// Arrange
	dir := rulesyncProject(t)

	// Act
	report, err := Convert(context.Background(), ConvertOptions{Source: dir, From: []string{"native,rulesync"}})

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "native,rulesync", report.Importer)
	assert.Nil(t, findingForReport(report, StatusDropped, "(native files)"))
}

func findingForReport(r *Report, status Status, source string) *Finding {
	for i := range r.Findings {
		if r.Findings[i].Status == status && r.Findings[i].Source == source {
			return &r.Findings[i]
		}
	}
	return nil
}

func TestConvert_RulesyncDryRunWritesNothingAndValidates(t *testing.T) {
	// Arrange
	dir := rulesyncProject(t)

	// Act
	report, err := Convert(context.Background(), ConvertOptions{Source: dir, From: []string{"rulesync"}})

	// Assert
	require.NoError(t, err)
	assert.False(t, report.Written)
	assert.Zero(t, report.Validation.Errors, "%v", report.Validation.Messages)
	assert.False(t, report.Security.Blocked)
	_, statErr := os.Stat(filepath.Join(dir, ".ai-rulez"))
	assert.True(t, os.IsNotExist(statErr), "dry run must not create .ai-rulez")
}

func TestConvert_RulesyncWrittenTreeLoadsAndValidates(t *testing.T) {
	// Arrange
	dir := rulesyncProject(t)

	// Act
	report, err := Convert(context.Background(), ConvertOptions{Source: dir, From: []string{"rulesync"}, Write: true})
	require.NoError(t, err)
	require.True(t, report.Written)
	loaded, loadErr := config.LoadConfigFromDir(context.Background(), dir, DefaultConfigDir, config.WithoutLocal(), config.WithoutRemote())

	// Assert
	require.NoError(t, loadErr)
	require.NoError(t, loaded.Validate())
	require.NotNil(t, loaded.Content)
	assert.Len(t, loaded.Content.Rules, 4)
	assert.Len(t, loaded.Content.Context, 1)
	assert.Len(t, loaded.Content.Agents, 1)
	assert.Len(t, loaded.Content.Skills, 1)
	assert.Len(t, loaded.Content.Checks, 1)
}

func TestConvert_RulesyncSecretBlocksTheWrite(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{
		".rulesync/rules/deploy.md": "Deploy with " + awsKey + " and " + ghToken + "\n",
	})

	// Act
	report, err := Convert(context.Background(), ConvertOptions{Source: dir, From: []string{"rulesync"}, Write: true})

	// Assert
	require.NoError(t, err)
	assert.True(t, report.Security.Blocked)
	assert.False(t, report.Written)
	_, statErr := os.Stat(filepath.Join(dir, ".ai-rulez"))
	assert.True(t, os.IsNotExist(statErr), "a blocked scan writes nothing")
}

func TestConvert_RulesyncReportConformsToSchema(t *testing.T) {
	// Arrange
	schemaBytes, err := os.ReadFile(filepath.Join("..", "..", "schema", "convert-report.schema.json"))
	require.NoError(t, err)
	schema, err := jsonschema.NewCompiler().Compile(schemaBytes)
	require.NoError(t, err)
	dir := rulesyncProject(t)
	report, err := Convert(context.Background(), ConvertOptions{Source: dir, From: []string{"rulesync"}})
	require.NoError(t, err)

	// Act
	var buf bytes.Buffer
	require.NoError(t, report.WriteJSON(&buf))

	// Assert
	result := schema.Validate(buf.Bytes())
	assert.True(t, result.IsValid(), "%v", result.Errors)
}

func TestConvert_RulesyncIsDeterministic(t *testing.T) {
	// Arrange
	run := func() string {
		dir := rulesyncProject(t)
		report, err := Convert(context.Background(), ConvertOptions{Source: dir, From: []string{"rulesync"}})
		require.NoError(t, err)
		var lines []string
		for _, f := range report.Findings {
			lines = append(lines, strings.Join([]string{string(f.Status), f.Source, f.Field, f.Target, f.Reason}, "|"))
		}
		return strings.Join(lines, "\n")
	}

	// Act
	first, second := run(), run()

	// Assert
	assert.Equal(t, first, second)
}

func TestRulesyncPlan_InputRootsEdgeCases(t *testing.T) {
	tests := []struct {
		name       string
		config     string
		wantRels   []string
		wantReport string // field of the unsupported finding, "" for none
		wantItems  int    // overrides reported as approximated
	}{
		{"a string where a list is expected", `{"inputRoots":"overlay/.rulesync"}`, []string{"rules/base.md"}, "inputRoots", 0},
		{"a non-string inputRoot", `{"inputRoot":42}`, []string{"rules/base.md"}, "inputRoot", 0},
		{"a non-string entry in the list", `{"inputRoots":[".rulesync", 7]}`, []string{"rules/base.md"}, "inputRoots", 0},
		{"the project root is refused", `{"inputRoots":[".", "./"]}`, []string{"rules/base.md"}, "inputRoots", 0},
		{"a repeated root is read once", `{"inputRoots":[".rulesync", "./.rulesync", ".rulesync/"]}`, []string{"rules/base.md"}, "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := map[string]string{
				"rulesync.jsonc":          tt.config,
				".rulesync/rules/base.md": "Base.\n",
				"README.md":               "not a rule\n",
			}

			p := rulesyncPlan(t, files)

			assert.Equal(t, tt.wantRels, itemRels(p))
			if tt.wantReport != "" {
				assert.NotNil(t, findingFor(p, StatusUnsupported, "rulesync.jsonc", tt.wantReport))
			}
			for i := range p.Findings {
				assert.NotEqual(t, StatusApproximated, p.Findings[i].Status, "a repeated root must not report itself as an override: %v", p.Findings[i])
			}
		})
	}
}

package importer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/kaptinlin/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mapFS(files map[string]string) fstest.MapFS {
	m := fstest.MapFS{}
	for p, c := range files {
		m[p] = &fstest.MapFile{Data: []byte(c)}
	}
	return m
}

func planOf(t *testing.T, f Format, fsys fs.FS, opt Options) *Plan {
	t.Helper()
	p, err := f.Plan(fsys, opt)
	require.NoError(t, err)
	p.Finalize()
	return p
}

func itemRels(p *Plan) []string {
	var out []string
	for i := range p.Items {
		out = append(out, p.Items[i].Rel())
	}
	sort.Strings(out)
	return out
}

func findingFor(p *Plan, status Status, source, field string) *Finding {
	for i := range p.Findings {
		f := &p.Findings[i]
		if f.Status == status && f.Source == source && (field == "" || f.Field == field) {
			return f
		}
	}
	return nil
}

func TestNativePlan_Layouts(t *testing.T) {
	tests := []struct {
		name        string
		files       map[string]string
		wantItems   []string
		wantPresets []string
	}{
		{
			name:        "claude files",
			files:       map[string]string{"CLAUDE.md": "# Project\n\nUse Go.\n", ".claude/agents/reviewer.md": "---\ndescription: r\n---\nReview.\n", ".claude/skills/lint/SKILL.md": "---\nname: lint\ndescription: lint it\n---\nRun lint.\n", ".claude/commands/build.md": "Build it.\n"},
			wantItems:   []string{"agents/reviewer.md", "commands/build.md", "context/claude.md", "skills/lint/SKILL.md"},
			wantPresets: []string{"claude"},
		},
		{
			name:        "cursor rules",
			files:       map[string]string{".cursor/rules/ts.mdc": "---\nglobs: **/*.ts\n---\nUse strict mode.\n"},
			wantItems:   []string{"rules/ts.md"},
			wantPresets: []string{"cursor"},
		},
		{
			name:        "copilot instructions",
			files:       map[string]string{".github/copilot-instructions.md": "Be brief.\n", ".github/instructions/go.instructions.md": "---\napplyTo: \"**/*.go\"\n---\nWrap errors.\n", ".github/prompts/fix.prompt.md": "Fix it.\n"},
			wantItems:   []string{"commands/fix.md", "context/copilot-instructions.md", "rules/go.md"},
			wantPresets: []string{"copilot"},
		},
		{
			name:        "kiro steering",
			files:       map[string]string{".kiro/steering/product.md": "---\ninclusion: always\n---\nProduct goals.\n"},
			wantItems:   []string{"rules/product.md"},
			wantPresets: []string{"kiro"},
		},
		{
			name:        "windsurf maps to devin",
			files:       map[string]string{".windsurf/rules/style.md": "---\ntrigger: always_on\n---\nStyle.\n", ".windsurf/workflows/deploy.md": "Deploy.\n"},
			wantItems:   []string{"commands/deploy.md", "rules/style.md"},
			wantPresets: []string{"devin"},
		},
		{
			name:        "roo and cline",
			files:       map[string]string{".roo/rules/a.md": "Roo rule.\n", ".clinerules/b.md": "Cline rule.\n", ".clinerules/workflows/w.md": "Workflow.\n"},
			wantItems:   []string{"commands/w.md", "rules/a.md", "rules/b.md"},
			wantPresets: []string{"cline", "zoocode"},
		},
		{
			name:        "qwen gemini junie",
			files:       map[string]string{"QWEN.md": "Qwen.\n", "GEMINI.md": "Gemini.\n", ".junie/guidelines.md": "Junie.\n"},
			wantItems:   []string{"context/gemini.md", "context/guidelines.md", "context/qwen.md"},
			wantPresets: []string{"gemini", "junie", "qwen"},
		},
		{
			name:        "shared files imply no preset",
			files:       map[string]string{"AGENTS.md": "Shared.\n", ".agents/skills/x/SKILL.md": "---\ndescription: x\n---\nX.\n"},
			wantItems:   []string{"context/agents.md", "skills/x/SKILL.md"},
			wantPresets: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			fsys := mapFS(tt.files)
			// Act
			p := planOf(t, nativeImporter{}, fsys, Options{})
			// Assert
			assert.Equal(t, tt.wantItems, itemRels(p))
			assert.Equal(t, tt.wantPresets, p.Presets)
		})
	}
}

func TestNativePlan_RuleFrontmatter(t *testing.T) {
	tests := []struct {
		name         string
		path         string
		content      string
		wantContains []string
		wantAbsent   []string
		wantFinding  *Finding
	}{
		{
			name:         "cursor globs and description",
			path:         ".cursor/rules/a.mdc",
			content:      "---\ndescription: TS rules\nglobs: *.ts, *.tsx\nalwaysApply: false\n---\nBody\n",
			wantContains: []string{"description: TS rules", "activation: glob", "- '*.ts'", "- '*.tsx'", "Body"},
		},
		{
			name:         "cursor always apply",
			path:         ".cursor/rules/a.mdc",
			content:      "---\nalwaysApply: true\n---\nBody\n",
			wantContains: []string{"Body"},
			wantAbsent:   []string{"activation"},
		},
		{
			name:         "cursor description only is approximated as auto",
			path:         ".cursor/rules/a.mdc",
			content:      "---\ndescription: when asked\n---\nBody\n",
			wantContains: []string{"activation: auto"},
			wantFinding:  &Finding{Status: StatusApproximated, Field: "description"},
		},
		{
			name:         "cursor without frontmatter is manual",
			path:         ".cursor/rules/a.mdc",
			content:      "Body\n",
			wantContains: []string{"activation: manual"},
			wantFinding:  &Finding{Status: StatusApproximated, Field: "frontmatter"},
		},
		{
			name:         "copilot applyTo",
			path:         ".github/instructions/a.instructions.md",
			content:      "---\napplyTo: \"src/**,lib/**\"\nexcludeAgent: code-review\n---\nBody\n",
			wantContains: []string{"activation: glob", "src/**", "lib/**"},
			wantFinding:  &Finding{Status: StatusDropped, Field: "excludeAgent"},
		},
		{
			name:         "kiro fileMatch",
			path:         ".kiro/steering/a.md",
			content:      "---\ninclusion: fileMatch\nfileMatchPattern: \"**/*.py\"\n---\nBody\n",
			wantContains: []string{"activation: glob", "**/*.py"},
		},
		{
			name:         "devin model decision",
			path:         ".devin/rules/a.md",
			content:      "---\ntrigger: model_decision\ndescription: use when testing\n---\nBody\n",
			wantContains: []string{"activation: auto", "description: use when testing"},
		},
		{
			name:         "unknown key is dropped",
			path:         ".claude/rules/a.md",
			content:      "---\npaths:\n  - src/**\nfancy: 1\n---\nBody\n",
			wantContains: []string{"src/**"},
			wantFinding:  &Finding{Status: StatusDropped, Field: "fancy"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := planOf(t, nativeImporter{}, mapFS(map[string]string{tt.path: tt.content}), Options{})
			require.Len(t, p.Items, 1)
			got := string(p.Items[0].Main)
			for _, s := range tt.wantContains {
				assert.Contains(t, got, s)
			}
			for _, s := range tt.wantAbsent {
				assert.NotContains(t, got, s)
			}
			if tt.wantFinding != nil {
				assert.NotNil(t, findingFor(p, tt.wantFinding.Status, tt.path, tt.wantFinding.Field), "finding %v in %v", tt.wantFinding, p.Findings)
			}
		})
	}
}

func TestNativePlan_SkipsGeneratedAndPointers(t *testing.T) {
	fsys := mapFS(map[string]string{
		"CLAUDE.md":                   "<!-- 🤖 AI-RULEZ :: GENERATED FILE — DO NOT EDIT -->\n# x\n",
		"GEMINI.md":                   "@AGENTS.md\n",
		"AGENTS.md":                   "Real content.\n",
		".cursor/rules/own.mdc":       "<!-- AI-RULEZ :: GENERATED -->\nbody\n",
		".claude/skills/s/SKILL.md":   "---\ndescription: s\n---\nS\n",
		".claude/skills/s/refs/a.txt": "ref",
	})
	p := planOf(t, nativeImporter{}, fsys, Options{})
	assert.Equal(t, []string{"context/agents.md", "skills/s/SKILL.md"}, itemRels(p))
	assert.NotNil(t, findingFor(p, StatusDropped, "CLAUDE.md", ""))
	assert.NotNil(t, findingFor(p, StatusDropped, "GEMINI.md", ""))
	assert.NotNil(t, findingFor(p, StatusDropped, ".cursor/rules/own.mdc", ""))
	require.Len(t, p.Items[1].Resources, 1)
	assert.Equal(t, "refs/a.txt", p.Items[1].Resources[0].Path)
}

func TestNativePlan_DedupeAndCollision(t *testing.T) {
	fsys := mapFS(map[string]string{
		"CLAUDE.md":              "Same text.\n",
		"AGENTS.md":              "Same text.\n",
		".cursor/rules/one.mdc":  "---\nalwaysApply: true\n---\nA\n",
		".roo/rules/one.md":      "B\n",
		".windsurf/rules/one.md": "A\n",
	})
	p := planOf(t, nativeImporter{}, fsys, Options{})
	var contexts, rules int
	for _, it := range p.Items {
		switch it.Kind {
		case KindContext:
			contexts++
		case KindRule:
			rules++
		}
	}
	assert.Equal(t, 1, contexts, "identical root files are one context")
	assert.Equal(t, 2, rules, "same name, different content: two rules, one renamed")
	renamed := 0
	for _, f := range p.Findings {
		if f.Status == StatusApproximated && f.Field == "name" {
			renamed++
		}
	}
	assert.GreaterOrEqual(t, renamed, 1)
}

func TestNativePlan_SplitHeadings(t *testing.T) {
	src := "Intro.\n\n## Build\n\nmake\n\n```\n## not a heading\n```\n\n## Test\n\ngo test\n"
	p := planOf(t, nativeImporter{}, mapFS(map[string]string{"CLAUDE.md": src}), Options{SplitHeadings: true})
	assert.Equal(t, []string{"context/claude-build.md", "context/claude-test.md", "context/claude.md"}, itemRels(p))
}

func TestNativePlan_RejectsSymlinksAndOversize(t *testing.T) {
	fsys := fstest.MapFS{
		"CLAUDE.md": &fstest.MapFile{Data: []byte(strings.Repeat("x", maxFileBytes+1))},
		"AGENTS.md": &fstest.MapFile{Mode: fs.ModeSymlink, Data: []byte("/etc/passwd")},
		".cursor":   &fstest.MapFile{Mode: fs.ModeSymlink, Data: []byte("../outside")},
	}
	p := planOf(t, nativeImporter{}, fsys, Options{})
	assert.Empty(t, p.Items)
}

func TestNativePlan_MCP(t *testing.T) {
	fsys := mapFS(map[string]string{
		".mcp.json": `{"mcpServers":{
			"github":{"command":"npx","args":["-y","gh-mcp"],"env":{"GITHUB_TOKEN":"ghp_literal","LOG":"debug","REF":"${HOME}"}},
			"remote":{"type":"http","url":"https://x.example/mcp","headers":{"Authorization":"Bearer abc"}},
			"ai":{"command":"npx","args":["-y","ai-rulez@latest","mcp"]},
			"odd":{"command":"x","timeout":5}
		}}`,
		".cursor/mcp.json": `{"mcpServers":{"github":{"command":"npx","args":["-y","gh-mcp"],"env":{"GITHUB_TOKEN":"ghp_literal","LOG":"debug","REF":"${HOME}"}}}}`,
	})
	p := planOf(t, nativeImporter{}, fsys, Options{})
	names := []string{}
	byName := map[string]string{}
	for _, s := range p.MCPServers {
		names = append(names, s.Name)
		byName[s.Name] = s.Command + s.URL
	}
	assert.Equal(t, []string{"github", "odd", "remote"}, names)
	for _, s := range p.MCPServers {
		if s.Name == "github" {
			assert.Equal(t, "${GITHUB_TOKEN}", s.Env["GITHUB_TOKEN"])
			assert.Equal(t, "debug", s.Env["LOG"])
			assert.Equal(t, "${HOME}", s.Env["REF"])
		}
		if s.Name == "remote" {
			assert.Equal(t, "http", s.Transport)
			assert.Equal(t, "Bearer ${AUTHORIZATION}", s.Headers["Authorization"])
		}
	}
	assert.NotNil(t, findingFor(p, StatusNeedsAction, ".mcp.json", "mcpServers.github.env.GITHUB_TOKEN"))
	assert.NotNil(t, findingFor(p, StatusDropped, ".mcp.json", "mcpServers.odd.timeout"))
	assert.NotNil(t, findingFor(p, StatusDropped, ".mcp.json", "mcpServers.ai"))
	for _, f := range p.Findings {
		assert.NotContains(t, f.Reason, "ghp_literal", "secret values are never reported")
	}
}

func TestNativePlan_ClaudeSettingsNeedAction(t *testing.T) {
	p := planOf(t, nativeImporter{}, mapFS(map[string]string{
		"CLAUDE.md":             "x\n",
		".claude/settings.json": `{"hooks":{"PreToolUse":[]},"permissions":{"allow":[]}}`,
	}), Options{})
	assert.NotNil(t, findingFor(p, StatusNeedsAction, ".claude/settings.json", "hooks"))
	assert.NotNil(t, findingFor(p, StatusNeedsAction, ".claude/settings.json", "permissions"))
}

const lockV1 = `{"version":1,"skills":{
 "alpha":{"source":"acme/skills","sourceType":"github","ref":"main","skillPath":"skills/alpha/SKILL.md","computedHash":"abc"},
 "beta":{"source":"acme/other","sourceType":"github","skillPath":"tools/beta/SKILL.md","computedHash":"def","subagents":["x"]},
 "gamma":{"source":"@scope/pkg","sourceType":"node_modules","computedHash":"1"},
 "delta":{"source":"./local","sourceType":"local","computedHash":"2"},
 "eps":{"source":"https://git.example/x/y.git","sourceUrl":"https://git.example/x/y.git","sourceType":"git","ref":"v1","skillPath":"SKILL.md","computedHash":"3"}
}}`

func TestSkillsLock_Plan(t *testing.T) {
	p := planOf(t, skillsLockImporter{}, mapFS(map[string]string{"skills-lock.json": lockV1}), Options{})
	got := map[string][3]string{}
	for _, s := range p.InstalledSkills {
		got[s.Name] = [3]string{s.Source, s.Ref, s.Path}
	}
	assert.Equal(t, map[string][3]string{
		"alpha": {"https://github.com/acme/skills", "main", ""},
		"beta":  {"https://github.com/acme/other", "", "tools/beta"},
		"eps":   {"https://git.example/x/y.git", "v1", "."},
	}, got)
	assert.NotNil(t, findingFor(p, StatusNeedsAction, "skills-lock.json", "skills.alpha.computedHash"))
	assert.NotNil(t, findingFor(p, StatusNeedsAction, "skills-lock.json", "skills.beta.ref"))
	assert.NotNil(t, findingFor(p, StatusDropped, "skills-lock.json", "skills.beta.subagents"))
	assert.NotNil(t, findingFor(p, StatusUnsupported, "skills-lock.json", "skills.gamma"))
	assert.NotNil(t, findingFor(p, StatusUnsupported, "skills-lock.json", "skills.delta"))
}

func TestSkillsLock_VersionAndInvalid(t *testing.T) {
	fsys := mapFS(map[string]string{"skills-lock.json": `{"version":2,"skills":{"a":{"source":"o/r","sourceType":"github","computedHash":"x"}}}`})
	_, err := skillsLockImporter{}.Plan(fsys, Options{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "version 2")

	p := planOf(t, skillsLockImporter{}, fsys, Options{BestEffort: true})
	assert.Len(t, p.InstalledSkills, 1)
	assert.NotNil(t, findingFor(p, StatusNeedsAction, "skills-lock.json", "version"))

	_, err = skillsLockImporter{}.Plan(mapFS(map[string]string{"skills-lock.json": "{nope"}), Options{})
	require.Error(t, err)
}

// --- Convert end to end ---

func writeTree(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for p, c := range files {
		full := filepath.Join(dir, filepath.FromSlash(p))
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(c), 0o644))
	}
}

func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	require.NoError(t, filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, rerr := os.ReadFile(p)
		require.NoError(t, rerr)
		rel, _ := filepath.Rel(dir, p)
		out[filepath.ToSlash(rel)] = string(data)
		return nil
	}))
	return out
}

var sampleProject = map[string]string{
	"CLAUDE.md":                    "# Project\n\nUse Go.\n",
	".cursor/rules/ts.mdc":         "---\nglobs: **/*.ts\n---\nUse strict mode.\n",
	".claude/skills/lint/SKILL.md": "---\nname: lint\ndescription: Run the linter\n---\nRun lint.\n",
	".mcp.json":                    `{"mcpServers":{"gh":{"command":"npx","args":["gh"],"env":{"GITHUB_TOKEN":"literal"}}}}`,
}

func TestConvert_DryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, sampleProject)
	before := snapshot(t, dir)

	report, err := Convert(context.Background(), ConvertOptions{Source: dir})

	require.NoError(t, err)
	assert.False(t, report.Written)
	assert.Equal(t, before, snapshot(t, dir))
	assert.Equal(t, 0, report.Validation.Errors, "%v", report.Validation.Messages)
	assert.False(t, report.Security.Blocked)
	assert.Contains(t, filePaths(report), "config.toml")
	assert.Contains(t, filePaths(report), "rules/ts.md")
}

func filePaths(r *Report) []string {
	var out []string
	for _, f := range r.Files {
		out = append(out, f.Path)
	}
	return out
}

func TestConvert_WriteIsIdempotentAndLeavesSourceAlone(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, sampleProject)

	first, err := Convert(context.Background(), ConvertOptions{Source: dir, Write: true})
	require.NoError(t, err)
	assert.True(t, first.Written)
	afterFirst := snapshot(t, dir)
	for p, c := range sampleProject {
		assert.Equal(t, c, afterFirst[p], "source file %s must be untouched", p)
	}
	config := afterFirst[".ai-rulez/config.toml"]
	assert.Contains(t, config, `presets = ['claude', 'cursor']`)
	assert.Contains(t, config, "${GITHUB_TOKEN}")
	assert.NotContains(t, config, "literal")

	second, err := Convert(context.Background(), ConvertOptions{Source: dir, Write: true})
	require.NoError(t, err)
	for _, f := range second.Files {
		assert.Equal(t, ActionUnchanged, f.Action, f.Path)
	}
	assert.Equal(t, afterFirst, snapshot(t, dir))
}

func TestConvert_ConflictNeedsForce(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, sampleProject)
	writeTree(t, dir, map[string]string{".ai-rulez/rules/ts.md": "mine\n", ".ai-rulez/config.toml": "version = \"4.0\"\nname = \"mine\"\npresets = [\"codex\"]\n"})
	before := snapshot(t, dir)

	report, err := Convert(context.Background(), ConvertOptions{Source: dir, Write: true})

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrConflicts))
	assert.Equal(t, before, snapshot(t, dir), "nothing is written on conflict")
	require.NotNil(t, report)
	assert.Equal(t, 1, report.Conflicts())

	forced, err := Convert(context.Background(), ConvertOptions{Source: dir, Write: true, Force: true})
	require.NoError(t, err)
	assert.True(t, forced.Written)
	after := snapshot(t, dir)
	assert.NotEqual(t, "mine\n", after[".ai-rulez/rules/ts.md"])
	assert.Contains(t, after[".ai-rulez/config.toml"], `'cursor'`)
	assert.Contains(t, after[".ai-rulez/config.toml"], "name = 'mine'", "--force does not replace config.toml")
	assert.Contains(t, after[".ai-rulez/config.toml"], `'codex'`)
}

func TestConvert_DomainKeepsExistingTree(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, sampleProject)
	writeTree(t, dir, map[string]string{".ai-rulez/config.toml": "version = \"4.0\"\nname = \"mine\"\npresets = [\"claude\"]\n", ".ai-rulez/rules/ts.md": "mine\n"})

	_, err := Convert(context.Background(), ConvertOptions{Source: dir, Write: true, Domain: "imported"})

	require.NoError(t, err)
	after := snapshot(t, dir)
	assert.Equal(t, "mine\n", after[".ai-rulez/rules/ts.md"])
	assert.Contains(t, after, ".ai-rulez/domains/imported/rules/ts.md")
}

func TestConvert_SecurityScanBlocksWrite(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{
		"CLAUDE.md": "Deploy with key AKIAIOSFODNN7EXAMPLE and ghp_" + strings.Repeat("a", 36) + "\n",
	})

	report, err := Convert(context.Background(), ConvertOptions{Source: dir, Write: true})

	require.NoError(t, err)
	assert.True(t, report.Security.Blocked)
	assert.Equal(t, CodeBlockedScan, report.Security.Code)
	assert.False(t, report.Written)
	_, statErr := os.Stat(filepath.Join(dir, ".ai-rulez"))
	assert.True(t, os.IsNotExist(statErr), "nothing is written when the scan blocks")
}

func TestConvert_SkillsLockWithNativeSkipsTrackedSkills(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{
		"skills-lock.json":              lockV1,
		".agents/skills/alpha/SKILL.md": "---\ndescription: a\n---\nA\n",
		".agents/skills/mine/SKILL.md":  "---\ndescription: m\n---\nM\n",
	})

	_, err := Convert(context.Background(), ConvertOptions{Source: dir})
	require.NoError(t, err, "auto runs every detected importer")

	report, err := Convert(context.Background(), ConvertOptions{Source: dir, From: []string{"native,skills-lock"}, Write: true})
	require.NoError(t, err)
	assert.True(t, report.Written)
	after := snapshot(t, dir)
	assert.Contains(t, after, ".ai-rulez/skills/mine/SKILL.md")
	assert.NotContains(t, after, ".ai-rulez/skills/alpha/SKILL.md")
	assert.Contains(t, after[".ai-rulez/config.toml"], "[[installed_skills]]")
	assert.Contains(t, after[".ai-rulez/config.toml"], "https://github.com/acme/skills")
}

func TestConvert_NothingToConvert(t *testing.T) {
	_, err := Convert(context.Background(), ConvertOptions{Source: t.TempDir()})
	require.Error(t, err)
}

func TestConvert_UnknownImporter(t *testing.T) {
	_, err := Convert(context.Background(), ConvertOptions{Source: t.TempDir(), From: []string{"nonesuch"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown importer")
}

func TestReport_JSONIsDeterministicAndCounted(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, sampleProject)

	run := func() []byte {
		report, err := Convert(context.Background(), ConvertOptions{Source: "." + "", Into: ".ai-rulez"})
		_ = report
		_ = err
		r, err := Convert(context.Background(), ConvertOptions{Source: dir})
		require.NoError(t, err)
		var buf bytes.Buffer
		require.NoError(t, r.WriteJSON(&buf))
		return buf.Bytes()
	}
	a, b := run(), run()
	assert.Equal(t, a, b)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(a, &decoded))
	assert.EqualValues(t, ReportSchemaVersion, decoded["schema_version"])
	counts := decoded["counts"].(map[string]any)
	assert.Greater(t, counts["mapped"].(float64), float64(0))
	assert.Contains(t, decoded, "security")
	assert.Contains(t, decoded, "validation")
}

func TestReport_MatchesFailOn(t *testing.T) {
	r := &Report{Findings: []Finding{{Status: StatusNeedsAction}, {Status: StatusMapped}}}
	assert.True(t, r.Matches([]string{"needs-action"}))
	assert.True(t, r.Matches([]string{"needs_action"}))
	assert.False(t, r.Matches([]string{"dropped", "unsupported"}))
}

// TestConvert_Golden converts fixtures under testdata/convert/<case>/in and
// compares the written tree and the findings with <case>/want. Run with
// UPDATE_GOLDEN=1 to regenerate after an intended change.
func TestConvert_Golden(t *testing.T) {
	cases, err := filepath.Glob(filepath.Join("testdata", "convert", "*"))
	require.NoError(t, err)
	require.NotEmpty(t, cases)
	for _, c := range cases {
		t.Run(filepath.Base(c), func(t *testing.T) {
			// Arrange
			dir := t.TempDir()
			copyDir(t, filepath.Join(c, "in"), dir)
			from := []string{"auto"}
			if data, rerr := os.ReadFile(filepath.Join(c, "from")); rerr == nil {
				from = []string{strings.TrimSpace(string(data))}
			}

			// Act
			report, err := Convert(context.Background(), ConvertOptions{Source: dir, From: from, Write: true})

			// Assert
			require.NoError(t, err)
			require.True(t, report.Written, "%+v", report.Security)
			got := snapshot(t, filepath.Join(dir, ".ai-rulez"))
			// The project name is the temp directory; pin it.
			cfg := strings.ReplaceAll(got["config.toml"], "name = '"+filepath.Base(dir)+"'", "name = 'fixture'")
			got["config.toml"] = cfg
			var lines []string
			for _, f := range report.Findings {
				lines = append(lines, strings.Join([]string{string(f.Status), f.Source, f.Field, f.Target, f.Reason}, " | "))
			}
			got["report.findings"] = strings.Join(lines, "\n") + "\n"

			want := filepath.Join(c, "want")
			if os.Getenv("UPDATE_GOLDEN") != "" {
				require.NoError(t, os.RemoveAll(want))
				for p, content := range got {
					full := filepath.Join(want, filepath.FromSlash(p))
					require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
					require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
				}
			}
			assert.Equal(t, snapshot(t, want), got)
		})
	}
}

func copyDir(t *testing.T, src, dst string) {
	t.Helper()
	require.NoError(t, filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		return os.WriteFile(target, data, 0o644)
	}))
}

func TestReport_ConformsToSchema(t *testing.T) {
	schemaBytes, err := os.ReadFile(filepath.Join("..", "..", "schema", "convert-report.schema.json"))
	require.NoError(t, err)
	schema, err := jsonschema.NewCompiler().Compile(schemaBytes)
	require.NoError(t, err)

	dir := t.TempDir()
	writeTree(t, dir, sampleProject)
	writeTree(t, dir, map[string]string{"GEMINI.md": "@AGENTS.md\n"})
	report, err := Convert(context.Background(), ConvertOptions{Source: dir})
	require.NoError(t, err)
	var buf bytes.Buffer
	require.NoError(t, report.WriteJSON(&buf))

	result := schema.Validate(buf.Bytes())
	assert.True(t, result.IsValid(), "%v", result.Errors)
}

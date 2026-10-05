package importer

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	ghToken = "ghp_" + "abcdefghijklmnopqrstuvwxyz0123456789"
	awsKey  = "AKIA" + "IOSFODNN7EXAMPLE"
)

func TestReportCodes_OwnRange(t *testing.T) {
	// Arrange
	want := map[Status]string{
		StatusApproximated: "AR9F1", StatusDropped: "AR9F2", StatusNeedsAction: "AR9F3", StatusUnsupported: "AR9F4",
	}

	// Act / Assert
	for status, code := range want {
		assert.Equal(t, code, newFinding(status, "x", "", "", "").Code, status)
	}
	assert.Equal(t, "AR9F5", CodeBlockedScan)
	assert.Empty(t, newFinding(StatusMapped, "x", "", "", "").Code)
}

func TestReport_SchemaAcceptsEveryCode(t *testing.T) {
	// Arrange
	schemaBytes, err := os.ReadFile(filepath.Join("..", "..", "schema", "convert-report.schema.json"))
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(schemaBytes, &doc))
	props := doc["properties"].(map[string]any)
	findingCodes := props["findings"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)["code"].(map[string]any)["enum"].([]any)
	secCode := props["security"].(map[string]any)["properties"].(map[string]any)["code"].(map[string]any)["const"]

	// Assert
	assert.ElementsMatch(t, []any{"AR9F1", "AR9F2", "AR9F3", "AR9F4"}, findingCodes)
	assert.Equal(t, "AR9F5", secCode)
}

// --- config.toml is merged, never replaced ---

func TestConvert_MergesExistingConfig(t *testing.T) {
	existing := "version = \"4.0\"\nname = \"mine\"\npresets = [\"codex\"]\n\n" +
		"[[mcp_servers]]\nname = \"gh\"\ncommand = \"mine-gh\"\n\n" +
		"[[installed_skills]]\nname = \"old\"\nsource = \"https://example.com/o/old\"\n"
	tests := []struct {
		name  string
		force bool
	}{{"without force", false}, {"with force", true}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := t.TempDir()
			writeTree(t, dir, sampleProject)
			writeTree(t, dir, map[string]string{".ai-rulez/config.toml": existing})

			// Act
			report, err := Convert(context.Background(), ConvertOptions{Source: dir, Write: true, Force: tt.force})

			// Assert
			require.NoError(t, err)
			assert.True(t, report.Written)
			cfgText := snapshot(t, dir)[".ai-rulez/config.toml"]
			assert.Contains(t, cfgText, "name = 'mine'")
			assert.Contains(t, cfgText, "'codex'")
			assert.Contains(t, cfgText, "'claude'")
			assert.Contains(t, cfgText, "'cursor'")
			assert.Contains(t, cfgText, "mine-gh", "an existing server of the same name is kept")
			assert.NotContains(t, cfgText, "npx", "the imported server of the same name does not replace it")
			assert.Contains(t, cfgText, "old")
			assert.NotNil(t, findingFor(&Plan{Findings: report.Findings}, StatusNeedsAction, "config.toml", ""), "the skipped server is reported")

			again, err := Convert(context.Background(), ConvertOptions{Source: dir, Write: true, Force: tt.force})
			require.NoError(t, err)
			for _, f := range again.Files {
				assert.Equal(t, ActionUnchanged, f.Action, f.Path)
			}
		})
	}
}

func TestConvert_MergeAppendsNewServersAndSkills(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{
		"skills-lock.json":      lockV1,
		".mcp.json":             `{"mcpServers":{"fresh":{"command":"npx","args":["x"]}}}`,
		".ai-rulez/config.toml": "version = \"4.0\"\nname = \"mine\"\npresets = [\"claude\"]\n\n[[mcp_servers]]\nname = \"gh\"\ncommand = \"g\"\n",
	})

	// Act
	_, err := Convert(context.Background(), ConvertOptions{Source: dir, Write: true})

	// Assert
	require.NoError(t, err)
	cfgText := snapshot(t, dir)[".ai-rulez/config.toml"]
	assert.Contains(t, cfgText, "name = 'gh'")
	assert.Contains(t, cfgText, "name = 'fresh'")
	assert.Contains(t, cfgText, "name = 'alpha'")
}

func TestConvert_ConfigInAnotherFormatIsLeftAlone(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	writeTree(t, dir, sampleProject)
	yamlCfg := "version: \"4.0\"\nname: mine\npresets: [claude]\n"
	writeTree(t, dir, map[string]string{".ai-rulez/config.yaml": yamlCfg})

	// Act
	report, err := Convert(context.Background(), ConvertOptions{Source: dir, Write: true, Force: true})

	// Assert
	require.NoError(t, err)
	after := snapshot(t, dir)
	assert.Equal(t, yamlCfg, after[".ai-rulez/config.yaml"])
	assert.NotContains(t, after, ".ai-rulez/config.toml", "a config.toml beside config.yaml would shadow it")
	assert.Contains(t, after, ".ai-rulez/rules/ts.md")
	assert.NotNil(t, findingFor(&Plan{Findings: report.Findings}, StatusNeedsAction, "config.yaml", ""))
}

func TestConvert_UnparsableConfigStopsTheRun(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	writeTree(t, dir, sampleProject)
	writeTree(t, dir, map[string]string{".ai-rulez/config.toml": "this is = = not toml ["})
	before := snapshot(t, dir)

	// Act
	_, err := Convert(context.Background(), ConvertOptions{Source: dir, Write: true, Force: true})

	// Assert
	require.Error(t, err)
	assert.Equal(t, before, snapshot(t, dir), "nothing is written")
}

// --- credentials ---

func mcpPlan(t *testing.T, server string) (*Plan, config.MCPServer) {
	t.Helper()
	p := planOf(t, nativeImporter{}, mapFS(map[string]string{".mcp.json": `{"mcpServers":{"s":` + server + `}}`}), Options{})
	require.Len(t, p.MCPServers, 1, "%v", p.Findings)
	return p, p.MCPServers[0]
}

func TestMCPCredentials(t *testing.T) {
	tests := []struct {
		name     string
		server   string
		check    func(t *testing.T, s config.MCPServer)
		wantLeak []string
		wantNeed bool
	}{
		{
			name:   "env value with a neutral key and a token value",
			server: `{"command":"x","env":{"CONFIG_BLOB":"` + ghToken + `","LOG":"debug"}}`,
			check: func(t *testing.T, s config.MCPServer) {
				assert.Equal(t, "${CONFIG_BLOB}", s.Env["CONFIG_BLOB"])
				assert.Equal(t, "debug", s.Env["LOG"])
			},
			wantLeak: []string{ghToken}, wantNeed: true,
		},
		{
			name:     "connection string in env",
			server:   `{"command":"x","env":{"DATABASE_URL":"postgres://app:hunter22@db.example/app"}}`,
			check:    func(t *testing.T, s config.MCPServer) { assert.Equal(t, "${DATABASE_URL}", s.Env["DATABASE_URL"]) },
			wantLeak: []string{"hunter22"}, wantNeed: true,
		},
		{
			name:   "plain connection string without a password is kept",
			server: `{"command":"x","env":{"DATABASE_URL":"postgres://db.example/app"}}`,
			check: func(t *testing.T, s config.MCPServer) {
				assert.Equal(t, "postgres://db.example/app", s.Env["DATABASE_URL"])
			},
		},
		{
			name:     "header with a neutral name and a token",
			server:   `{"url":"https://x.example/mcp","type":"http","headers":{"X-Custom":"` + ghToken + `"}}`,
			check:    func(t *testing.T, s config.MCPServer) { assert.Equal(t, "${X_CUSTOM}", s.Headers["X-Custom"]) },
			wantLeak: []string{ghToken}, wantNeed: true,
		},
		{
			name:   "basic auth header",
			server: `{"url":"https://x.example/mcp","type":"http","headers":{"Authorization":"Basic dXNlcjpwYXNz"}}`,
			check: func(t *testing.T, s config.MCPServer) {
				assert.Equal(t, "Basic ${AUTHORIZATION}", s.Headers["Authorization"])
			},
			wantLeak: []string{"dXNlcjpwYXNz"}, wantNeed: true,
		},
		{
			name:   "bearer reference is kept",
			server: `{"url":"https://x.example/mcp","type":"http","headers":{"Authorization":"Bearer ${MY_TOKEN}"}}`,
			check: func(t *testing.T, s config.MCPServer) {
				assert.Equal(t, "Bearer ${MY_TOKEN}", s.Headers["Authorization"])
			},
		},
		{
			name:   "api key flag with a separate value",
			server: `{"command":"x","args":["--port","8080","--api-key","s3cr3tvalue99"]}`,
			check: func(t *testing.T, s config.MCPServer) {
				assert.Equal(t, []string{"--port", "8080", "--api-key", "${API_KEY}"}, s.Args)
			},
			wantLeak: []string{"s3cr3tvalue99"}, wantNeed: true,
		},
		{
			name:     "token flag with equals",
			server:   `{"command":"x","args":["--token=abc123def"]}`,
			check:    func(t *testing.T, s config.MCPServer) { assert.Equal(t, []string{"--token=${TOKEN}"}, s.Args) },
			wantLeak: []string{"abc123def"}, wantNeed: true,
		},
		{
			name:   "docker style env argument",
			server: `{"command":"docker","args":["run","-e","API_KEY=abcdef123456","img"]}`,
			check: func(t *testing.T, s config.MCPServer) {
				assert.Equal(t, []string{"run", "-e", "API_KEY=${API_KEY}", "img"}, s.Args)
			},
			wantLeak: []string{"abcdef123456"}, wantNeed: true,
		},
		{
			name:     "token literal as an argument",
			server:   `{"command":"x","args":["` + ghToken + `"]}`,
			check:    func(t *testing.T, s config.MCPServer) { assert.NotContains(t, s.Args[0], "ghp_") },
			wantLeak: []string{ghToken}, wantNeed: true,
		},
		{
			name:   "userinfo in the url",
			server: `{"url":"https://user:pw1234@x.example/mcp","type":"http"}`,
			check: func(t *testing.T, s config.MCPServer) {
				assert.Equal(t, "https://${S_URL_USERINFO}@x.example/mcp", s.URL)
			},
			wantLeak: []string{"pw1234"}, wantNeed: true,
		},
		{
			name:   "credential query parameter",
			server: `{"url":"https://x.example/mcp?v=1&api_key=qq998877&x=2","type":"http"}`,
			check: func(t *testing.T, s config.MCPServer) {
				assert.Equal(t, "https://x.example/mcp?v=1&api_key=${API_KEY}&x=2", s.URL)
			},
			wantLeak: []string{"qq998877"}, wantNeed: true,
		},
		{
			name:   "existing references are kept",
			server: `{"command":"x","env":{"A_TOKEN":"${A_TOKEN}","B_KEY":"$B_KEY"}}`,
			check: func(t *testing.T, s config.MCPServer) {
				assert.Equal(t, "${A_TOKEN}", s.Env["A_TOKEN"])
				assert.Equal(t, "$B_KEY", s.Env["B_KEY"])
			},
		},
		{
			name:     "a reference embedded in a literal does not excuse it",
			server:   `{"command":"x","env":{"A_TOKEN":"lit3ral${HOME}s3cret"}}`,
			check:    func(t *testing.T, s config.MCPServer) { assert.Equal(t, "${A_TOKEN}", s.Env["A_TOKEN"]) },
			wantLeak: []string{"lit3ral"}, wantNeed: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange / Act
			p, s := mcpPlan(t, tt.server)

			// Assert
			tt.check(t, s)
			haveNeed := false
			for _, f := range p.Findings {
				if f.Status == StatusNeedsAction {
					haveNeed = true
				}
				for _, leak := range tt.wantLeak {
					assert.NotContains(t, f.Reason+f.Field+f.Target, leak, "findings never carry the value")
				}
			}
			assert.Equal(t, tt.wantNeed, haveNeed)
			rendered, err := config.MarshalTOML(&config.Config{Version: "4.0", Name: "x", MCPServersRaw: p.MCPServers})
			require.NoError(t, err)
			for _, leak := range tt.wantLeak {
				assert.NotContains(t, string(rendered), leak)
			}
		})
	}
}

func TestConvert_ScanCoversConfigToml(t *testing.T) {
	// Arrange: the description is copied into config.toml, where only the
	// config scan can see it.
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{".mcp.json": `{"mcpServers":{"s":{"command":"x","description":"deploy key ` + awsKey + `"}}}`})

	// Act
	report, err := Convert(context.Background(), ConvertOptions{Source: dir, Write: true})

	// Assert
	require.NoError(t, err)
	assert.True(t, report.Security.Blocked)
	assert.False(t, report.Written)
	found := false
	for _, f := range report.Security.Findings {
		if f.File == ".ai-rulez/config.toml" || f.File == "config.toml" {
			found = true
		}
		assert.NotContains(t, f.Message, awsKey)
	}
	assert.True(t, found, "%+v", report.Security.Findings)
}

func TestConvert_DryRunReportsSecurityEvenWhenValidationFails(t *testing.T) {
	// Arrange: a skill and a command with one name collide on their output id.
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{
		".claude/skills/dup/SKILL.md": "---\nname: dup\ndescription: d\n---\nBody\n",
		".claude/commands/dup.md":     "Command body\n",
		"CLAUDE.md":                   "key " + awsKey + "\n",
	})

	// Act
	report, err := Convert(context.Background(), ConvertOptions{Source: dir})

	// Assert
	require.NoError(t, err)
	require.Positive(t, report.Validation.Errors, "fixture must fail validation")
	assert.True(t, report.Security.Blocked, "%+v", report.Security)
	assert.NotEmpty(t, report.Security.Findings)
}

func TestConvert_SecurityScanIgnoresInlineSuppression(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"CLAUDE.md": "<!-- ai-rulez-lint-ignore AR001 -->\nkey " + awsKey + "\n"})

	// Act
	report, err := Convert(context.Background(), ConvertOptions{Source: dir})

	// Assert
	require.NoError(t, err)
	assert.True(t, report.Security.Blocked)
}

// --- skills-lock sources ---

func lockWith(source, ref, skillPath string) string {
	b, _ := json.Marshal(map[string]any{"version": 1, "skills": map[string]any{
		"s": map[string]any{"source": source, "sourceType": "git", "ref": ref, "skillPath": skillPath},
	}})
	return string(b)
}

func TestSkillsLock_RejectsUnsafeFields(t *testing.T) {
	tests := []struct {
		name, source, ref, skillPath string
		wantOK                       bool
	}{
		{"https", "https://github.com/o/r", "main", "skills/s/SKILL.md", true},
		{"ssh scheme", "ssh://git@github.com/o/r.git", "", "SKILL.md", true},
		{"scp style", "git@github.com:o/r.git", "v1", "skills/s/SKILL.md", true},
		{"ext transport", "ext::sh -c touch% /tmp/x", "", "", false},
		{"leading dash", "-oProxyCommand=x://y", "", "", false},
		{"option as scheme-ful source", "--upload-pack=x://y", "", "", false},
		{"file scheme", "file:///etc", "", "", false},
		{"plain http", "http://example.com/o/r", "", "", false},
		{"git protocol", "git://example.com/o/r", "", "", false},
		{"credentials in url", "https://user:pw@example.com/o/r", "", "", false},
		{"ref option", "https://github.com/o/r", "--output=/tmp/x", "", false},
		{"dotdot skill path", "https://github.com/o/r", "", "../../etc/SKILL.md", false},
		{"absolute skill path", "https://github.com/o/r", "", "/etc/SKILL.md", false},
		{"nested dotdot", "https://github.com/o/r", "", "a/../../b/SKILL.md", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			fsys := mapFS(map[string]string{skillsLockFile: lockWith(tt.source, tt.ref, tt.skillPath)})

			// Act
			p := planOf(t, skillsLockImporter{}, fsys, Options{})

			// Assert
			if tt.wantOK {
				require.Len(t, p.InstalledSkills, 1, "%v", p.Findings)
				return
			}
			assert.Empty(t, p.InstalledSkills)
			assert.NotNil(t, findingFor(p, StatusUnsupported, skillsLockFile, ""), "%v", p.Findings)
			for _, f := range p.Findings {
				assert.NotContains(t, f.Reason, "pw@")
			}
		})
	}
}

func TestCheckStaged_ValidatesInstalledSkills(t *testing.T) {
	// Arrange
	cfg := &config.Config{
		Version: config.ConfigVersionV4, Name: "x", Presets: []config.Preset{{BuiltIn: "claude"}},
		InstalledSkills: []config.InstalledSkillConfig{{Name: "s", Source: "--upload-pack=x", Path: "../x"}},
	}
	report := &Report{}
	files := map[string][]byte{"config.toml": []byte("version = \"4.0\"\nname = \"x\"\npresets = [\"claude\"]\n")}

	// Act
	err := checkStaged(context.Background(), report, files, cfg)

	// Assert
	require.NoError(t, err)
	assert.Positive(t, report.Validation.Errors)
}

// --- generated files ---

func TestNativePlan_SkipsGeneratedMarkersEverywhere(t *testing.T) {
	// Arrange
	fsys := mapFS(map[string]string{
		".claude/skills/s/SKILL.md":       "---\ndescription: s\n---\nS\n",
		".claude/skills/s/run.sh":         "#!/bin/sh\n# Generated by ai-rulez from skills/s. Edit the source.\necho hi\n",
		".claude/skills/s/ref.md":         "<!-- 🤖 AI-RULEZ :: GENERATED FILE — DO NOT EDIT -->\nref\n",
		".claude/skills/s/keep.txt":       "mine\n",
		".cursor/rules/a.mdc":             "---\nalwaysApply: true\n---\n<!-- GENERATED FILE -->\nbody\n",
		".github/copilot-instructions.md": "<!-- Generated by ai-rulez -->\nbody\n",
	})

	// Act
	p := planOf(t, nativeImporter{}, fsys, Options{})

	// Assert
	assert.Equal(t, []string{"skills/s/SKILL.md"}, itemRels(p))
	require.Len(t, p.Items[0].Resources, 1)
	assert.Equal(t, "keep.txt", p.Items[0].Resources[0].Path)
	assert.NotNil(t, findingFor(p, StatusDropped, ".claude/skills/s/run.sh", ""))
}

func generatedProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{
		".ai-rulez/config.toml":                "version = \"4.0\"\nname = \"gen\"\npresets = [\"claude\", \"cursor\", \"copilot\", \"gemini\"]\n",
		".ai-rulez/rules/style.md":             "---\ndescription: style\n---\nUse tabs.\n",
		".ai-rulez/context/overview.md":        "Overview text.\n",
		".ai-rulez/skills/lint/SKILL.md":       "---\nname: lint\ndescription: Run the linter\n---\nRun lint.\n",
		".ai-rulez/skills/lint/scripts/run.sh": "#!/bin/sh\necho lint\n",
		".ai-rulez/agents/reviewer.md":         "---\ndescription: reviews\n---\nReview.\n",
	})
	cfg, err := config.LoadConfigFromDir(context.Background(), dir, ".ai-rulez", config.WithoutLocal(), config.WithoutRemote())
	require.NoError(t, err)
	require.NoError(t, generator.NewGenerator(cfg).Generate(""))
	require.Contains(t, snapshot(t, dir), "CLAUDE.md")
	return dir
}

func TestConvert_GeneratedTreeIsNotReimported(t *testing.T) {
	t.Run("with the generate manifest nothing is imported", func(t *testing.T) {
		// Arrange
		dir := generatedProject(t)

		// Act
		_, err := Convert(context.Background(), ConvertOptions{Source: dir})

		// Assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "nothing to convert")
	})
	t.Run("without the manifest the generated headers still apply", func(t *testing.T) {
		// Arrange
		dir := generatedProject(t)
		require.NoError(t, os.RemoveAll(filepath.Join(dir, ".ai-rulez")))

		// Act
		report, err := Convert(context.Background(), ConvertOptions{Source: dir})

		// Assert
		if err != nil {
			assert.Contains(t, err.Error(), "nothing to convert")
			return
		}
		for _, f := range report.Files {
			assert.False(t, strings.HasPrefix(f.Path, "skills/"), "a generated skill was re-imported: %v", report.Files)
			assert.False(t, strings.HasPrefix(f.Path, "context/"), "a generated root file was re-imported: %v", report.Files)
			assert.False(t, strings.HasPrefix(f.Path, "rules/"), "a generated rule was re-imported: %v", report.Files)
		}
	})
}

// --- auto runs every detected importer ---

func TestConvert_AutoRunsAllDetectedImporters(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{
		"skills-lock.json":              lockV1,
		".agents/skills/alpha/SKILL.md": "---\ndescription: a\n---\nA\n",
		".agents/skills/mine/SKILL.md":  "---\ndescription: m\n---\nM\n",
	})

	// Act
	report, err := Convert(context.Background(), ConvertOptions{Source: dir, Write: true})

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "skills-lock,native", report.Importer)
	after := snapshot(t, dir)
	assert.Contains(t, after, ".ai-rulez/skills/mine/SKILL.md")
	assert.NotContains(t, after, ".ai-rulez/skills/alpha/SKILL.md", "a skill the lock tracks is not also copied")
	assert.Contains(t, after[".ai-rulez/config.toml"], "[[installed_skills]]")
}

// --- read errors are reported ---

type denyFS struct {
	fs.FS
	deny map[string]bool
}

func (d denyFS) Open(name string) (fs.File, error) {
	if d.deny[name] {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrPermission}
	}
	return d.FS.Open(name)
}

func TestNativePlan_ReportsReadErrors(t *testing.T) {
	// Arrange
	base := mapFS(map[string]string{
		"CLAUDE.md":                 "ok\n",
		".cursor/rules/a.mdc":       "---\nalwaysApply: true\n---\nA\n",
		".claude/skills/s/SKILL.md": "---\ndescription: s\n---\nS\n",
		".claude/settings.json":     `{"hooks": `,
		".kiro/steering/b.md":       "B\n",
		".github/instructions/c.md": "C\n",
	})
	fsys := denyFS{FS: base, deny: map[string]bool{".cursor/rules": true, ".kiro/steering": true, ".github/instructions/c.md": true}}

	// Act
	p := planOf(t, nativeImporter{}, fsys, Options{})

	// Assert
	assert.NotNil(t, findingFor(p, StatusDropped, ".cursor/rules", ""), "%v", p.Findings)
	assert.NotNil(t, findingFor(p, StatusDropped, ".kiro/steering", ""), "%v", p.Findings)
	assert.NotNil(t, findingFor(p, StatusDropped, ".github/instructions/c.md", ""), "%v", p.Findings)
	f := findingFor(p, StatusDropped, ".claude/settings.json", "")
	require.NotNil(t, f, "%v", p.Findings)
	assert.Contains(t, f.Reason, "JSON")
}

func TestNativePlan_ReportsSymlinkedSources(t *testing.T) {
	// Arrange
	fsys := fstest.MapFS{
		".cursor/rules": &fstest.MapFile{Mode: fs.ModeSymlink, Data: []byte("../outside")},
		"CLAUDE.md":     &fstest.MapFile{Data: []byte("ok\n")},
	}

	// Act
	p := planOf(t, nativeImporter{}, fsys, Options{})

	// Assert
	f := findingFor(p, StatusDropped, ".cursor/rules", "")
	require.NotNil(t, f, "%v", p.Findings)
	assert.Contains(t, f.Reason, "symlink")
}

// --- write safety ---

func skipWithoutSymlinks(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on windows")
	}
}

func TestConvert_RefusesSymlinkedTargets(t *testing.T) {
	skipWithoutSymlinks(t)
	tests := []struct {
		name  string
		setup func(t *testing.T, dir, outside string)
	}{
		{"config dir is a symlink", func(t *testing.T, dir, outside string) {
			require.NoError(t, os.Symlink(outside, filepath.Join(dir, ".ai-rulez")))
		}},
		{"content dir is a symlink", func(t *testing.T, dir, outside string) {
			require.NoError(t, os.MkdirAll(filepath.Join(dir, ".ai-rulez"), 0o755))
			require.NoError(t, os.Symlink(outside, filepath.Join(dir, ".ai-rulez", "context")))
		}},
		{"target file is a symlink", func(t *testing.T, dir, outside string) {
			require.NoError(t, os.MkdirAll(filepath.Join(dir, ".ai-rulez", "context"), 0o755))
			require.NoError(t, os.Symlink(filepath.Join(outside, "victim"), filepath.Join(dir, ".ai-rulez", "context", "claude.md")))
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir, outside := t.TempDir(), t.TempDir()
			writeTree(t, dir, map[string]string{"CLAUDE.md": "# Project\n"})
			writeTree(t, outside, map[string]string{"victim": "keep\n"})
			tt.setup(t, dir, outside)

			// Act
			_, err := Convert(context.Background(), ConvertOptions{Source: dir, Write: true, Force: true})

			// Assert
			require.Error(t, err)
			assert.Contains(t, err.Error(), "symlink")
			assert.Equal(t, map[string]string{"victim": "keep\n"}, snapshot(t, outside), "nothing is written through the link")
		})
	}
}

func TestConvert_RollbackRemovesCreatedDirectories(t *testing.T) {
	// Arrange: a directory sits where the skill file must go, so the last write fails.
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{
		"CLAUDE.md":                    "# Project\n",
		".claude/skills/lint/SKILL.md": "---\ndescription: l\n---\nL\n",
	})
	blocker := filepath.Join(dir, ".ai-rulez", "skills", "lint", "SKILL.md")
	require.NoError(t, os.MkdirAll(blocker, 0o755))

	// Act
	_, err := Convert(context.Background(), ConvertOptions{Source: dir, Write: true, Force: true})

	// Assert
	require.Error(t, err)
	var left []string
	require.NoError(t, filepath.WalkDir(filepath.Join(dir, ".ai-rulez"), func(p string, d fs.DirEntry, werr error) error {
		rel, _ := filepath.Rel(dir, p)
		left = append(left, filepath.ToSlash(rel))
		return werr
	}))
	assert.Equal(t, []string{".ai-rulez", ".ai-rulez/skills", ".ai-rulez/skills/lint", ".ai-rulez/skills/lint/SKILL.md"}, left,
		"directories created by the failed run are removed")
}

// --- frontmatter of agents, commands and skills ---

func TestNativePlan_AgentFrontmatter(t *testing.T) {
	tests := []struct {
		name         string
		path, source string
		wantContains []string
		wantAbsent   []string
		wantFields   []string
	}{
		{
			name: "claude tools string becomes a list; tool specific keys are reported",
			path: ".claude/agents/r.md", source: "---\nname: r\ndescription: reviews\ntools: Read, Grep, Glob\nmodel: opus\ncolor: blue\npermissionMode: plan\n---\nReview.\n",
			wantContains: []string{"- Read", "- Grep", "- Glob", "model: opus"},
			wantAbsent:   []string{"tools: Read, Grep"},
			wantFields:   []string{"tools", "color", "permissionMode"},
		},
		{
			name: "copilot agent keys",
			path: ".github/agents/r.agent.md", source: "---\ndescription: reviews\ntools: ['search']\nhandoffs:\n  - label: x\n    agent: y\ntarget: vscode\n---\nReview.\n",
			wantFields: []string{"handoffs", "target"},
		},
		{
			name: "copilot prompt keys",
			path: ".github/prompts/p.prompt.md", source: "---\nmode: agent\ndescription: fix\nmodel: gpt-5\nagent: reviewer\n---\nFix it.\n",
			wantFields: []string{"mode", "agent"},
		},
		{
			name: "plain files need no report",
			path: ".claude/commands/c.md", source: "Do it.\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange / Act
			p := planOf(t, nativeImporter{}, mapFS(map[string]string{tt.path: tt.source}), Options{})

			// Assert
			require.Len(t, p.Items, 1)
			got := string(p.Items[0].Main)
			for _, s := range tt.wantContains {
				assert.Contains(t, got, s)
			}
			for _, s := range tt.wantAbsent {
				assert.NotContains(t, got, s)
			}
			for _, field := range tt.wantFields {
				f := findingFor(p, StatusApproximated, tt.path, field)
				assert.NotNil(t, f, "finding for %s in %v", field, p.Findings)
			}
			if len(tt.wantFields) == 0 {
				for _, f := range p.Findings {
					assert.Equal(t, StatusMapped, f.Status, "%v", f)
				}
			}
		})
	}
}

func TestNativePlan_SkillNameFollowsDirectory(t *testing.T) {
	// Arrange
	fsys := mapFS(map[string]string{
		".claude/skills/My Skill/SKILL.md":  "---\nname: My Skill\ndescription: d\n---\nBody\n",
		".agents/skills/Other_One/SKILL.md": "---\nname: \"Other_One\"\ndescription: d\n---\nBody2\n",
	})

	// Act
	p := planOf(t, nativeImporter{}, fsys, Options{})

	// Assert
	byName := map[string]string{}
	for _, it := range p.Items {
		byName[it.Name] = string(it.Main)
	}
	require.Contains(t, byName, "my-skill")
	assert.Contains(t, byName["my-skill"], "name: my-skill")
	assert.NotContains(t, byName["my-skill"], "My Skill")
	require.Contains(t, byName, "other_one")
	assert.Contains(t, byName["other_one"], "name: other_one")
}

func TestNativePlan_CollisionRenameUpdatesSkillName(t *testing.T) {
	// Arrange
	fsys := mapFS(map[string]string{
		".claude/skills/dup/SKILL.md": "---\nname: dup\ndescription: one\n---\nOne\n",
		".agents/skills/dup/SKILL.md": "---\nname: dup\ndescription: two\n---\nTwo\n",
	})

	// Act
	p := planOf(t, nativeImporter{}, fsys, Options{})

	// Assert
	require.Len(t, p.Items, 2)
	for _, it := range p.Items {
		assert.Contains(t, string(it.Main), "name: "+it.Name+"\n", "SKILL.md name matches %s", it.Name)
	}
}

// --- names ---

func TestNativePlan_NonASCIINames(t *testing.T) {
	// Arrange
	fsys := mapFS(map[string]string{
		".cursor/rules/規則.mdc":          "---\nalwaysApply: true\n---\nA\n",
		".cursor/rules/文書.mdc":          "---\nalwaysApply: true\n---\nB\n",
		".claude/skills/règle/SKILL.md": "---\ndescription: d\n---\nS\n",
		".claude/skills/日本語/SKILL.md":   "---\ndescription: d\n---\nT\n",
	})

	// Act
	p := planOf(t, nativeImporter{}, fsys, Options{})

	// Assert
	names := map[string]bool{}
	for _, it := range p.Items {
		assert.NotEqual(t, "unnamed", it.Name)
		assert.Regexp(t, `^[a-z0-9_-]+$`, it.Name)
		names[string(it.Kind)+it.Name] = true
	}
	assert.Len(t, names, 4, "distinct sources keep distinct names: %v", itemRels(p))
	assert.NotNil(t, findingFor(p, StatusApproximated, ".cursor/rules/規則.mdc", "name"), "%v", p.Findings)
}

func TestSplitH2_Fences(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []string
	}{
		{"backtick fence", "pre\n```\n## in\n```\n## Real\nx\n", []string{"", "Real"}},
		{"longer fence holds a shorter one", "````\n```\n## in\n````\n## Real\nx\n", []string{"", "Real"}},
		{"tilde fence holds backticks", "~~~\n```\n## in\n~~~\n## Real\nx\n", []string{"", "Real"}},
		{"backtick fence holds tildes", "```\n~~~\n## in\n```\n## Real\nx\n", []string{"", "Real"}},
		{"indented fence", "pre\n   ```\n## in\n   ```\n## Real\nx\n", []string{"", "Real"}},
		{"closing fence may be longer", "```\n## in\n`````\n## Real\nx\n", []string{"", "Real"}},
		{"info string on the closing line does not close", "```\n## in\n```go\n## still in\n```\n## Real\nx\n", []string{"", "Real"}},
		{"heading outside", "pre\n## A\na\n## B\nb\n", []string{"", "A", "B"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			var got []string
			for _, s := range splitH2(tt.text) {
				got = append(got, s.header)
			}

			// Assert
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestTranslateRule_AlwaysApplyWithGlobs(t *testing.T) {
	// Arrange
	p := planOf(t, nativeImporter{}, mapFS(map[string]string{
		".cursor/rules/a.mdc": "---\nalwaysApply: true\nglobs: src/**\ndescription: d\n---\nBody\n",
	}), Options{})

	// Assert
	require.Len(t, p.Items, 1)
	got := string(p.Items[0].Main)
	assert.NotContains(t, got, "src/**", "an always-on rule is not path scoped")
	assert.NotContains(t, got, "activation: glob")
	assert.NotNil(t, findingFor(p, StatusApproximated, ".cursor/rules/a.mdc", "globs"), "%v", p.Findings)
}

func TestParseFrontmatter_LenientFallback(t *testing.T) {
	tests := []struct {
		name        string
		fm          string
		want        map[string]any
		wantLenient bool
	}{
		{"valid yaml", "description: x\nglobs: \"**/*.ts\"\n", map[string]any{"description": "x", "globs": "**/*.ts"}, false},
		{"alias-looking glob with a block scalar", "description: |\n  line one\n  line two\nglobs: **/*.ts\n",
			map[string]any{"description": "line one\nline two\n", "globs": "**/*.ts"}, true},
		{"alias-looking glob with a folded scalar", "description: >-\n  line one\n  line two\nglobs: *.go\n",
			map[string]any{"description": "line one line two", "globs": "*.go"}, true},
		{"alias-looking glob with a list", "globs:\n  - **/*.ts\n  - *.go\ndescription: d\n",
			map[string]any{"globs": []any{"**/*.ts", "*.go"}, "description": "d"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got, lenient := parseFrontmatter(tt.fm)

			// Assert
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.wantLenient, lenient)
		})
	}
}

func TestNativePlan_LenientFrontmatterIsReported(t *testing.T) {
	// Arrange
	p := planOf(t, nativeImporter{}, mapFS(map[string]string{
		".cursor/rules/a.mdc": "---\ndescription: |\n  multi\n  line\nglobs: **/*.ts\n---\nBody\n",
	}), Options{})

	// Assert
	require.Len(t, p.Items, 1)
	got := string(p.Items[0].Main)
	assert.NotContains(t, got, "description: '|'")
	assert.NotContains(t, got, "description: |\n", "not a bare block marker without content")
	assert.Contains(t, got, "multi")
	assert.NotNil(t, findingFor(p, StatusApproximated, ".cursor/rules/a.mdc", "frontmatter"), "%v", p.Findings)
	assert.True(t, strings.Contains(got, "**/*.ts"))
}

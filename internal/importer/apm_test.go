package importer

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseAPMDep(t *testing.T) {
	tests := []struct {
		name       string
		in         any
		want       apmDep
		wantReason string
	}{
		{name: "owner/repo", in: "microsoft/apm-sample-package", want: apmDep{host: "github.com", owner: "microsoft", repo: "apm-sample-package"}},
		{name: "ref", in: "owner/repo#v1.0.0", want: apmDep{host: "github.com", owner: "owner", repo: "repo", ref: "v1.0.0"}},
		{name: "case is folded for the owner", in: "Owner/Repo", want: apmDep{host: "github.com", owner: "owner", repo: "Repo"}},
		{name: "virtual path", in: "anthropics/skills/skills/frontend-design#main", want: apmDep{host: "github.com", owner: "anthropics", repo: "skills", subpath: "skills/frontend-design", ref: "main"}},
		{name: "https url", in: "https://github.com/owner/repo.git#main", want: apmDep{host: "github.com", owner: "owner", repo: "repo", ref: "main"}},
		{name: "other host", in: "https://gitlab.com/group/repo", want: apmDep{host: "gitlab.com", owner: "group", repo: "repo"}},
		{name: "host shorthand", in: "gitlab.example.com/group/repo", want: apmDep{host: "gitlab.example.com", owner: "group", repo: "repo"}},
		{name: "object", in: map[string]any{"git": "https://github.com/acme/standards.git", "path": "instructions/security", "ref": "v2.0", "alias": "review"},
			want: apmDep{host: "github.com", owner: "acme", repo: "standards", subpath: "instructions/security", ref: "v2.0"}},
		{name: "local path", in: "./packages/pack", want: apmDep{local: "packages/pack"}},
		{name: "local path out of the project", in: "../elsewhere", wantReason: "must stay inside the project"},
		{name: "absolute path", in: "/etc/pack", wantReason: "outside the project"},
		{name: "ssh", in: "git@github.com:acme/x.git", wantReason: "SSH dependencies"},
		{name: "marketplace", in: "plugin@marketplace", wantReason: "marketplace"},
		{name: "http is refused", in: "http://github.com/owner/repo", wantReason: "only https"},
		{name: "credentials in the url", in: "https://user:pw@github.com/owner/repo", wantReason: "plain https repository URL"},
		{name: "path escapes the repository", in: map[string]any{"git": "owner/repo", "path": "../x"}, wantReason: ".. segments"},
		{name: "absolute object path", in: map[string]any{"git": "owner/repo", "path": "/x"}, wantReason: "relative"},
		{name: "object without git", in: map[string]any{"path": "x"}, wantReason: "git"},
		{name: "not owner/repo", in: "justone", wantReason: "owner/repo"},
		{name: "number", in: 3, wantReason: "neither a string nor a mapping"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got, reason := parseAPMDep(tt.in)
			// Assert
			if tt.wantReason != "" {
				assert.Contains(t, reason, tt.wantReason)
				return
			}
			assert.Empty(t, reason)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestAPMPlan_ProjectPrimitives(t *testing.T) {
	// Arrange
	fsys := mapFS(map[string]string{
		"apm.yml":                              "name: x\n",
		".apm/instructions/py.instructions.md": "---\napplyTo: \"**/*.py\"\ndescription: Py\n---\nTyped.\n",
		".apm/agents/r.agent.md":               "---\nname: r\ndescription: R\n---\nReview.\n",
		".apm/chatmodes/plan.chatmode.md":      "---\ndescription: P\n---\nPlan.\n",
		".apm/prompts/go.prompt.md":            "Go.\n",
		".apm/context/arch.context.md":         "Arch.\n",
		".apm/skills/s/SKILL.md":               "---\nname: s\ndescription: S\n---\nS.\n",
		".apm/unknown/x.md":                    "x\n",
	})
	// Act
	p := planOf(t, apmImporter{}, fsys, Options{})
	// Assert
	assert.Equal(t, []string{"agents/plan.md", "agents/r.md", "commands/go.md", "context/arch.md", "rules/py.md", "skills/s/SKILL.md"}, itemRels(p))
	assert.NotNil(t, findingFor(p, StatusDropped, ".apm/unknown", ""))
	var rule string
	for i := range p.Items {
		if p.Items[i].Kind == KindRule {
			rule = string(p.Items[i].Main)
		}
	}
	assert.Contains(t, rule, "globs:")
	assert.Contains(t, rule, "'**/*.py'")
}

func TestAPMPlan_DependencyHandling(t *testing.T) {
	// Arrange
	fsys := mapFS(map[string]string{
		"apm.yml": `dependencies:
  apm:
    - acme/installed#v1
    - acme/missing#v2
    - ./pack
    - git@github.com:acme/ssh.git
  mcp:
    - io.github.x/registry-server
    - {name: docs, transport: sse, url: "https://x.example/sse"}
    - {transport: http, url: "https://noname.example"}
`,
		"apm.lock.yaml": `lockfile_version: '1'
dependencies:
  - repo_url: https://github.com/acme/missing
    resolved_commit: fedcba9876543210fedcba9876543210fedcba98
    content_hash: sha256:abc
  - repo_url: https://github.com/acme/bad
    resolved_commit: not-a-commit
`,
		"apm_modules/acme/installed/.apm/instructions/a.instructions.md": "Installed rule.\n",
		"pack/.apm/prompts/p.prompt.md":                                  "Local prompt.\n",
	})
	// Act
	p := planOf(t, apmImporter{}, fsys, Options{})
	// Assert
	assert.Equal(t, []string{"commands/p.md", "rules/a.md"}, itemRels(p))
	require.Len(t, p.Remotes, 1)
	assert.Equal(t, Remote{
		Kind: remotePackage, Origin: "apm.yml#dependencies.apm[1]", URL: "https://github.com/acme/missing", Ref: "v2",
		Commit: "fedcba9876543210fedcba9876543210fedcba98",
	}, p.Remotes[0])
	assert.NotNil(t, findingFor(p, StatusUnsupported, "apm.yml", "dependencies.apm[3]"))
	assert.NotNil(t, findingFor(p, StatusUnsupported, "apm.yml", "dependencies.mcp[0]"), "registry servers are not resolved")
	assert.NotNil(t, findingFor(p, StatusUnsupported, "apm.yml", "dependencies.mcp[2]"), "a server needs a name")
	require.Len(t, p.MCPServers, 1)
	assert.Equal(t, "docs", p.MCPServers[0].Name)
	assert.Equal(t, "sse", string(p.MCPServers[0].Transport))
	assert.NotNil(t, findingFor(p, StatusNeedsAction, "apm.lock.yaml", "dependencies.content_hash"))
}

func TestAPMPlan_InstalledPackagesNotInTheManifestAreImported(t *testing.T) {
	// Arrange: a transitive dependency that apm.yml does not name.
	fsys := mapFS(map[string]string{
		"apm.yml": "name: x\n",
		"apm_modules/acme/transitive/.apm/instructions/t.instructions.md": "Transitive.\n",
		"apm_modules/acme/bundle/SKILL.md":                                "---\nname: bundle-skill\ndescription: A bundle\n---\nBundle.\n",
		"apm_modules/acme/bundle/references/doc.md":                       "Doc.\n",
		"apm_modules/acme/bundle/README.md":                               "Readme.\n",
		"apm_modules/acme/plugin/agents/helper.md":                        "---\ndescription: H\n---\nHelp.\n",
	})
	// Act
	p := planOf(t, apmImporter{}, fsys, Options{})
	// Assert
	assert.Equal(t, []string{"agents/helper.md", "rules/t.md", "skills/bundle-skill/SKILL.md"}, itemRels(p))
	for i := range p.Items {
		if p.Items[i].Kind == KindSkill {
			require.Len(t, p.Items[i].Resources, 1, "README and package files are not skill resources")
			assert.Equal(t, "references/doc.md", p.Items[i].Resources[0].Path)
		}
	}
}

func TestAPMPlan_InvalidManifestIsAnError(t *testing.T) {
	_, err := apmImporter{}.Plan(mapFS(map[string]string{"apm.yml": "dependencies: [unclosed\n"}), Options{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), CodeInvalid)
}

func TestAPMPlan_SymlinkedDependencyDirIsNotFollowed(t *testing.T) {
	// A MapFS has no symlinks; the reader's refusal is covered by importer_symlink_test.
	p := planOf(t, apmImporter{}, mapFS(map[string]string{"apm.yml": "dependencies:\n  apm:\n    - ./../escape\n"}), Options{})

	assert.NotNil(t, findingFor(p, StatusUnsupported, "apm.yml", "dependencies.apm[0]"))
	assert.Empty(t, p.Items)
}

func TestAPMPlan_HookScriptsAreReported(t *testing.T) {
	// Arrange
	fsys := mapFS(map[string]string{
		".apm/hooks/h.json":       `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"${CLAUDE_PLUGIN_ROOT}/hooks/run.sh"}]}]}}`,
		".apm/hooks/scripts/x.sh": "echo x\n",
	})
	// Act
	p := planOf(t, apmImporter{}, fsys, Options{})
	// Assert
	require.Len(t, p.Hooks, 1)
	assert.NotNil(t, findingFor(p, StatusDropped, ".apm/hooks/scripts/x.sh", ""))
	assert.NotNil(t, findingFor(p, StatusNeedsAction, ".apm/hooks/h.json", "hooks.Stop[0].hooks[0].command"))
}

func TestConvert_APMDetectionAndNativeSkip(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{
		"apm.yml":                             "name: x\n",
		".apm/instructions/a.instructions.md": "Rule.\n",
		"AGENTS.md":                           "Compiled.\n",
	})
	// Act
	report, err := Convert(context.Background(), ConvertOptions{Source: dir})
	// Assert
	require.NoError(t, err)
	assert.Equal(t, "apm", report.Importer)
	var dropped bool
	for _, f := range report.Findings {
		dropped = dropped || (f.Source == "(native files)" && strings.Contains(f.Reason, ".apm/"))
	}
	assert.True(t, dropped, "native is skipped next to APM and the report says why")
}

func TestConvert_RemoteOnlyProjectExplainsFetch(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"apm.yml": "dependencies:\n  apm:\n    - acme/missing\n"})

	_, err := Convert(context.Background(), ConvertOptions{Source: dir})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "nothing to convert")
}

func TestAPMTargets(t *testing.T) {
	p := planOf(t, apmImporter{}, mapFS(map[string]string{
		"apm.yml":                             "target: [vscode, claude, bogus, all]\n",
		".apm/instructions/a.instructions.md": "x\n",
	}), Options{})

	assert.Equal(t, []string{"claude", "copilot"}, p.Presets)
	assert.NotNil(t, findingFor(p, StatusUnsupported, "apm.yml", "target.bogus"))
	assert.NotNil(t, findingFor(p, StatusNeedsAction, "apm.yml", "target"))
}

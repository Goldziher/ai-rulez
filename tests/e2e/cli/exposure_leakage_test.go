package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// exposureLeakageConfig is an inline area map with a term and an ambiguous word, and one acceptance.
const exposureLeakageConfig = `
[exposure]
plugins = "none"

[[exposure.areas]]
name = "billing"
paths = ["services/billing"]
terms = ["invoice"]
ambiguous = ["card"]

[[lint.accept]]
rule = "AR9W0"
path = ".mcp.json"
match = 'mcp_server "invoice-'
reason = "server names stay in the root .mcp.json"
`

// leakageProject is exposureProject with start-of-session text that names the billing area: an MCP server, an
// unconditional root rule and a root skill whose description holds only the ambiguous word.
func leakageProject(t *testing.T, env *isoEnv, extraConfig string) string {
	t.Helper()
	root := exposureProject(t, env, extraConfig)
	writeTree(t, root, map[string]string{
		".mcp.json":                   `{"mcpServers": {"invoice-db": {"type": "http", "url": "https://example.com/mcp"}}}`,
		".claude/rules/billing.md":    "# Billing\n\nEvery invoice goes through the ledger.\n",
		".claude/skills/pay/SKILL.md": "---\nname: pay\ndescription: Takes a card payment.\n---\nbody\n",
	})
	return root
}

type leakageFinding struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Area     string `json:"area"`
	Term     string `json:"term"`
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	Path     string `json:"path"`
	Line     int    `json:"line"`
	Reason   string `json:"reason"`
}

type leakageDoc struct {
	Root             string           `json:"root"`
	ArtifactsChecked int              `json:"artifacts_checked"`
	Findings         []leakageFinding `json:"findings"`
	Stale            []string         `json:"stale_acceptances"`
}

func leakage(t *testing.T, stdout string) leakageDoc {
	t.Helper()
	validateAgainstSchema(t, "exposure-leakage.v1.schema.json", stdout)
	var doc leakageDoc
	require.NoError(t, json.Unmarshal([]byte(stdout), &doc), stdout)
	return doc
}

func TestExposureLeakage(t *testing.T) {
	env := newIsoEnv(t)
	root := leakageProject(t, env, exposureLeakageConfig)
	accepted := leakageFinding{Severity: "accepted", Code: "AR9W0", Area: "billing", Term: "invoice", Kind: "mcp_server", Name: "invoice-db", Path: ".mcp.json", Line: 1, Reason: "server names stay in the root .mcp.json"}
	rule := leakageFinding{Severity: "violation", Code: "AR9W0", Area: "billing", Term: "invoice", Kind: "rule", Name: "billing", Path: ".claude/rules/billing.md", Line: 3}
	skill := leakageFinding{Severity: "warning", Code: "AR9W1", Area: "billing", Term: "card", Kind: "skill", Name: "pay", Path: ".claude/skills/pay/SKILL.md", Line: 1}

	t.Run("json lists the violations, warnings and accepted findings and exits 2", func(t *testing.T) {
		res := env.run(root, "exposure", "leakage", "--format", "json")
		require.Equal(t, 2, res.ExitCode, "stdout: %s\nstderr: %s", res.Stdout, res.Stderr)
		doc := leakage(t, res.Stdout)
		assert.Equal(t, []leakageFinding{accepted, rule, skill}, doc.Findings)
		assert.Empty(t, doc.Stale)
		assert.Positive(t, doc.ArtifactsChecked)
	})

	t.Run("--plugins all checks the marketplace skills too", func(t *testing.T) {
		res := env.run(root, "exposure", "leakage", "--format", "json", "--plugins", "all")
		require.Equal(t, 2, res.ExitCode, res.Stderr)
		var names []string
		for _, f := range leakage(t, res.Stdout).Findings {
			names = append(names, f.Name)
		}
		assert.Contains(t, names, "ops:probe")
	})

	t.Run("text summarizes the findings", func(t *testing.T) {
		res := env.run(root, "exposure", "leakage")
		require.Equal(t, 2, res.ExitCode, res.Stderr)
		for _, want := range []string{"violations: 1  warnings: 1  accepted: 1", "violation AR9W0 billing/invoice .claude/rules/billing.md:3", "accepted: server names stay"} {
			assert.Contains(t, res.Stdout, want)
		}
	})

	t.Run("validate reports the same findings", func(t *testing.T) {
		res := env.run(root, "validate", "--analyzer", "exposure", "--format", "json")
		require.Equal(t, 2, res.ExitCode, res.Stderr)
		assert.Contains(t, res.Stdout, `rule \"billing\" at session start names term \"invoice\" of area \"billing\"`)
		assert.Contains(t, res.Stdout, `"AR9W1"`)
	})

	// Last: it adds a file below the project.
	t.Run("--root below the project inspects that directory, with paths relative to it", func(t *testing.T) {
		writeTree(t, root, map[string]string{"services/billing/CLAUDE.md": "# Billing\n\nEvery invoice is ours.\n"})
		billing := filepath.Join("services", "billing")

		res := env.run(root, "exposure", "leakage", "--format", "json", "--root", billing)

		require.Equal(t, 2, res.ExitCode, "stdout: %s\nstderr: %s", res.Stdout, res.Stderr)
		doc := leakage(t, res.Stdout)
		assert.True(t, strings.HasSuffix(doc.Root, "/services/billing"), "root %s", doc.Root)
		assert.Equal(t, []leakageFinding{
			{Severity: "violation", Code: "AR9W0", Area: "billing", Term: "invoice", Kind: "claude_md", Name: "CLAUDE.md", Path: "CLAUDE.md", Line: 3},
			{Severity: "warning", Code: "AR9W1", Area: "billing", Term: "card", Kind: "skill", Name: "charge", Path: ".claude/skills/charge/SKILL.md", Line: 1},
		}, doc.Findings, "the session of services/billing, not the repository root's")
		assert.Empty(t, doc.Stale, "the acceptance of the root .mcp.json is not judged below the root")

		text := env.run(root, "exposure", "leakage", "--root", billing)
		require.Equal(t, 2, text.ExitCode, text.Stderr)
		assert.Contains(t, text.Stdout, "violation AR9W0 billing/invoice CLAUDE.md:3")
	})
}

func TestExposureLeakageStaleAcceptance(t *testing.T) {
	env := newIsoEnv(t)
	root := leakageProject(t, env, exposureLeakageConfig+`
[[lint.accept]]
rule = "AR9W0"
path = ".claude/rules/billing.md"
match = 'names term "invoice"'
reason = "the rule is the billing pointer"

[[lint.accept]]
rule = "AR9W1"
path = "CLAUDE.md"
match = 'names term "card"'
reason = "gone"
`)

	res := env.run(root, "exposure", "leakage", "--format", "json")

	require.Equal(t, 2, res.ExitCode, "stdout: %s\nstderr: %s", res.Stdout, res.Stderr)
	doc := leakage(t, res.Stdout)
	assert.Equal(t, []string{"lint.accept[2] (AR9W1 in CLAUDE.md) accepted no finding; remove it or fix its path or match"}, doc.Stale)
	for _, f := range doc.Findings {
		assert.NotEqual(t, "violation", f.Severity, "%+v", f)
	}
}

func TestExposureLeakagePasses(t *testing.T) {
	env := newIsoEnv(t)
	root := exposureProject(t, env, "")
	writeTree(t, root, map[string]string{"clean.toml": "[[area]]\nname = \"search\"\nterms = [\"ranker\"]\n"})

	res := env.run(root, "exposure", "leakage", "--areas", "clean.toml", "--format", "json")

	require.Equal(t, 0, res.ExitCode, "stdout: %s\nstderr: %s", res.Stdout, res.Stderr)
	assert.Empty(t, leakage(t, res.Stdout).Findings)
}

func TestExposureLeakageWithoutAMap(t *testing.T) {
	env := newIsoEnv(t)
	root := exposureProject(t, env, "")

	res := env.run(root, "exposure", "leakage")

	require.Equal(t, 1, res.ExitCode, "stdout: %s\nstderr: %s", res.Stdout, res.Stderr)
	assert.Contains(t, res.Stderr, "no area map")
}

// codexLeakageProject names the billing area only in Codex-rendered start-of-session artifacts: the root
// AGENTS.md, a .agents/skills listing and a .codex/config.toml MCP server. No CLAUDE.md or .claude/ file exists.
func codexLeakageProject(t *testing.T, extraConfig string) string {
	t.Helper()
	root := minimalProject(t, "\n[exposure]\n"+extraConfig+`
[[exposure.areas]]
name = "billing"
paths = ["services/billing"]
terms = ["invoice"]
`)
	writeTree(t, root, map[string]string{
		"AGENTS.md":                      "# Repo\n\nEvery invoice goes through the ledger.\n",
		".agents/skills/ledger/SKILL.md": "---\nname: ledger\ndescription: Reconciles each invoice.\n---\nbody\n",
		".codex/config.toml":             "[mcp_servers.invoice-db]\ncommand = \"db\"\n",
	})
	return root
}

func TestExposureLeakageCodexHarness(t *testing.T) {
	env := newIsoEnv(t)
	agents := leakageFinding{Severity: "violation", Code: "AR9W0", Area: "billing", Term: "invoice", Kind: "agents_md", Name: "AGENTS.md", Path: "AGENTS.md", Line: 3}
	skill := leakageFinding{Severity: "violation", Code: "AR9W0", Area: "billing", Term: "invoice", Kind: "skill", Name: "ledger", Path: ".agents/skills/ledger/SKILL.md", Line: 1}
	server := leakageFinding{Severity: "violation", Code: "AR9W0", Area: "billing", Term: "invoice", Kind: "mcp_server", Name: "invoice-db", Path: ".codex/config.toml", Line: 1}

	t.Run("--harness codex checks the Codex inventory", func(t *testing.T) {
		root := codexLeakageProject(t, "")

		res := env.run(root, "exposure", "leakage", "--harness", "codex", "--format", "json")

		require.Equal(t, 2, res.ExitCode, "stdout: %s\nstderr: %s", res.Stdout, res.Stderr)
		assert.ElementsMatch(t, []leakageFinding{agents, skill, server}, leakage(t, res.Stdout).Findings)
	})

	t.Run("[exposure] harness = codex is the default", func(t *testing.T) {
		root := codexLeakageProject(t, "harness = \"codex\"\n")

		res := env.run(root, "exposure", "leakage", "--format", "json")

		require.Equal(t, 2, res.ExitCode, "stdout: %s\nstderr: %s", res.Stdout, res.Stderr)
		assert.ElementsMatch(t, []leakageFinding{agents, skill, server}, leakage(t, res.Stdout).Findings)

		claude := env.run(root, "exposure", "leakage", "--harness", "claude", "--format", "json")
		require.Equal(t, 0, claude.ExitCode, "--harness overrides the table\nstdout: %s\nstderr: %s", claude.Stdout, claude.Stderr)
		assert.Empty(t, leakage(t, claude.Stdout).Findings)
	})

	t.Run("the claude harness does not see Codex-only artifacts", func(t *testing.T) {
		root := codexLeakageProject(t, "")

		res := env.run(root, "exposure", "leakage", "--format", "json")

		require.Equal(t, 0, res.ExitCode, "stdout: %s\nstderr: %s", res.Stdout, res.Stderr)
		assert.Empty(t, leakage(t, res.Stdout).Findings)
	})

	t.Run("validate AR9W0 follows [exposure] harness", func(t *testing.T) {
		root := codexLeakageProject(t, "harness = \"codex\"\n")

		res := env.run(root, "validate", "--analyzer", "exposure", "--format", "json")

		require.Equal(t, 2, res.ExitCode, "stdout: %s\nstderr: %s", res.Stdout, res.Stderr)
		assert.Contains(t, res.Stdout, `skill \"ledger\" at session start names term \"invoice\" of area \"billing\"`)
		assert.Contains(t, res.Stdout, `agents_md \"AGENTS.md\" at session start names term \"invoice\"`)
	})

	t.Run("--plugins other than none and an unknown --harness are errors", func(t *testing.T) {
		root := codexLeakageProject(t, "")

		plugins := env.run(root, "exposure", "leakage", "--harness", "codex", "--plugins", "all")
		assert.Equal(t, 1, plugins.ExitCode, plugins.Stderr)
		assert.Contains(t, plugins.Stderr, "no marketplace plugins")

		bad := env.run(root, "exposure", "leakage", "--harness", "gemini")
		assert.Equal(t, 1, bad.ExitCode, bad.Stderr)
		assert.Contains(t, bad.Stderr, "unknown --harness")
	})
}

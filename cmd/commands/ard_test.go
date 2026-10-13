package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/ard"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/publish"
)

const ardProjectConfig = `version = "5.0"
name = "acme"
presets = ["claude"]

[ard]
publisher = "acme.test"
namespace = "conventions"

[ard.queries]
docs = ["search the acme docs", "find the api reference"]

[[mcp_servers]]
name = "docs"
description = "Search the docs."
transport = "http"
url = "https://docs.acme.test/mcp"
headers = { Authorization = "Bearer ${DOCS_TOKEN}" }

[[mcp_servers]]
name = "local-tools"
description = "Local helper."
command = "npx"
args = ["-y", "tools@1.2.3"]
env = { TOKEN = "${TOOLS_TOKEN}" }

[plugin]
name = "acme"
description = "Acme skills."
version = "1.4.0"
repository = "https://github.com/acme/skills"
runtimes = ["agent-plugins"]
keywords = ["conventions"]
category = "development"

[plugin.author]
name = "Jane"
`

// ardProject is a committed project with a skill that has triggers and eval
// cases, two MCP servers and a plugin.
func ardProject(t *testing.T) string {
	t.Helper()
	t.Setenv("DOCS_TOKEN", "docs-secret-value")
	t.Setenv("TOOLS_TOKEN", "tools-secret-value")
	root := publishProjectWith(t, ardProjectConfig)
	skill := filepath.Join(root, ".ai-rulez", "skills", "deploy")
	writeFile(t, filepath.Join(skill, "SKILL.md"),
		"---\nname: deploy\ndescription: Use when deploying the service to production.\nmetadata:\n  version: 1.2.0\nkeywords: [release, ops]\n"+
			"triggers: [deploy to production]\nrepresentative_queries:\n  - how do I deploy the api\n---\n\n# Deploy\n")
	writeFile(t, filepath.Join(skill, "evals", "deploy.eval.yaml"),
		"schema_version: 1\ncases:\n  - id: basic\n    prompt: ship the staging build\n    expect_trigger: true\n    near_miss:\n      - explain our environments\n"+
			"  - id: unrelated\n    prompt: what is the capital of France\n    expect_trigger: false\n")
	chdirRegenerate(t)
	return root
}

func chdirRegenerate(t *testing.T) {
	t.Helper()
	require.Equal(t, 0, runRecursiveGenerate(), "generate")
	cfg, err := config.LoadConfig(context.Background(), ".", config.WithoutLocal())
	require.NoError(t, err)
	require.NoError(t, generator.NewGenerator(cfg).GeneratePlugin(""))
	require.Equal(t, 0, writeLockAt("", "", nil), "lock")
	publishGit(t, ".", "add", "-A")
	publishGit(t, ".", "commit", "-q", "-m", "ard")
}

func TestPublish_EmitsTheARDManifest(t *testing.T) {
	// Arrange
	golden, err := filepath.Abs(filepath.Join("testdata", "ard", "ard.json"))
	require.NoError(t, err)
	root := ardProject(t)
	publishEmit = []string{"ard"}
	t.Setenv("SOURCE_DATE_EPOCH", "1790000000")

	// Act
	_, err = runPublishCapture(t)

	// Assert
	require.NoError(t, err)
	dist := readDist(t, filepath.Join(root, "dist"))
	got := dist["emit/ard/ard.json"]
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		require.NoError(t, os.WriteFile(golden, []byte(got), 0o600))
	}
	want, err := os.ReadFile(golden)
	require.NoError(t, err)
	assert.Equal(t, string(want), got, "run UPDATE_GOLDEN=1 go test ./cmd/commands -run TestPublish_EmitsTheARDManifest and review the diff")
	for _, leaked := range []string{"TOKEN", "secret-value", "Bearer"} {
		assert.NotContains(t, got, leaked, "env and header values never reach the public manifest")
	}
	found, err := ard.Validate([]byte(got))
	require.NoError(t, err)
	assert.False(t, ard.HasErrors(found), "%+v", found)
}

func TestPublishEmit_WritesOneServerCardPerRemoteServer(t *testing.T) {
	// Arrange
	ardProject(t)
	publishEmit = []string{"mcp-server-card"}
	publishEmitOut = filepath.Join(t.TempDir(), "cards")
	t.Setenv("SOURCE_DATE_EPOCH", "1790000000")
	var out bytes.Buffer
	var err error

	// Act
	_, _ = capture(t, func() { err = runPublishEmit(context.Background(), &out, "mcp-server-card") })

	// Assert
	require.NoError(t, err)
	files := readDist(t, publishEmitOut)
	require.Len(t, files, 1, "the stdio server has no card")
	card := files["docs.json"]
	assert.Contains(t, card, `"name": "test.acme/docs"`, "the namespace is the reversed [ard] publisher")
	assert.Contains(t, card, `"version": "1.4.0"`)
	assert.Contains(t, card, `"isSecret": true`)
	for _, leaked := range []string{"TOKEN", "secret-value", "Bearer", "local-tools"} {
		assert.NotContains(t, card, leaked)
	}
}

func TestPublish_EmitsTheServerCardsNextToTheARDManifest(t *testing.T) {
	root := ardProject(t)
	publishEmit = []string{"ard", "mcp-server-card"}
	t.Setenv("SOURCE_DATE_EPOCH", "1790000000")

	_, err := runPublishCapture(t)

	require.NoError(t, err)
	dist := readDist(t, filepath.Join(root, "dist"))
	var card map[string]any
	require.NoError(t, json.Unmarshal([]byte(dist["emit/mcp-server-card/docs.json"]), &card))
	assert.Contains(t, dist["emit/ard/ard.json"], `"name": "test.acme/docs"`, "the ARD embed and the standalone card agree")
}

func TestPublish_ARDManifestIsTheSameOnEveryRun(t *testing.T) {
	root := ardProject(t)
	publishEmit = []string{"ard"}
	t.Setenv("SOURCE_DATE_EPOCH", "1790000000")
	_, err := runPublishCapture(t)
	require.NoError(t, err)
	first := readDist(t, filepath.Join(root, "dist"))["emit/ard/ard.json"]
	require.NoError(t, os.RemoveAll(filepath.Join(root, "dist")))

	_, err = runPublishCapture(t)
	require.NoError(t, err)

	assert.Equal(t, first, readDist(t, filepath.Join(root, "dist"))["emit/ard/ard.json"])
}

func TestPublish_ARDEmitterNeedsTheARDTable(t *testing.T) {
	publishProject(t)
	publishEmit = []string{"ard"}
	publishDryRun = true

	_, err := runPublishCapture(t)

	requirePublishError(t, err, publish.CodeConfig, 1)
	assert.Contains(t, err.Error(), "AR9S4")
}

func TestPublishEmit_ExportsOnlyTheARDManifest(t *testing.T) {
	ardProject(t)
	publishEmit = []string{"ard"}
	publishEmitOut = filepath.Join(t.TempDir(), "ard")
	t.Setenv("SOURCE_DATE_EPOCH", "1790000000")
	var out bytes.Buffer
	var err error

	_, _ = capture(t, func() { err = runPublishEmit(context.Background(), &out, "ard") })

	require.NoError(t, err)
	files := readDist(t, publishEmitOut)
	assert.Len(t, files, 1)
	assert.Contains(t, files["ard.json"], "urn:air:acme.test:conventions:deploy")
}

func ardCodes(findings []lint.Finding, prefix string) map[string]lint.Severity {
	out := map[string]lint.Severity{}
	for _, f := range findings {
		if strings.HasPrefix(f.Code, prefix) {
			out[f.Code] = f.Severity
		}
	}
	return out
}

func TestStrictValidate_LintsTheARDManifest(t *testing.T) {
	ardProject(t)

	got := ardCodes(strictFindings(t), "AR9S")

	// local-tools is stdio, which a server card cannot describe, so it is not listed
	assert.Empty(t, got)
}

func TestStrictValidate_ReportsARDProblems(t *testing.T) {
	// a server named like the skill: two resources, one identifier
	cfg := ardProjectConfig + "\n[[mcp_servers]]\nname = \"deploy\"\ntransport = \"http\"\nurl = \"https://deploy.acme.test/mcp\"\n"
	t.Setenv("DOCS_TOKEN", "x")
	t.Setenv("TOOLS_TOKEN", "x")
	publishProjectWith(t, cfg)

	got := ardCodes(strictFindings(t), "AR9S")

	assert.Equal(t, lint.SeverityError, got[lint.CodeARDIdentifier], "%v", got)
}

func TestStrictValidate_IsQuietWithoutARD(t *testing.T) {
	publishProject(t)

	assert.Empty(t, ardCodes(strictFindings(t), "AR9S"))
}

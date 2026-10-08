package commands

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/ard"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
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
	for _, leaked := range []string{"TOKEN", "secret-value", "Authorization"} {
		assert.NotContains(t, got, leaked, "env values and headers never reach the public manifest")
	}
	found, err := ard.Validate([]byte(got))
	require.NoError(t, err)
	assert.False(t, ard.HasErrors(found), "%+v", found)
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

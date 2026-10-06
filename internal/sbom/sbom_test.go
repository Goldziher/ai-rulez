package sbom_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/kaptinlin/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/sbom"
)

const baseConfig = `version = "4.0"
name = "demo"
presets = ["claude"]
`

const mcpConfig = `
[[mcp_servers]]
name = "fs"
command = "npx"
args = ["-y", "@modelcontextprotocol/server-filesystem@1.2.3", "/tmp"]
env = { API_KEY = "env-secret-value" }

[[mcp_servers]]
name = "remote"
transport = "http"
url = "https://user:url-secret@example.com/mcp?key=query-secret#frag"
headers = { Authorization = "Bearer header-secret" }
`

type project struct {
	cfg string
	// files maps a path under .ai-rulez to its content.
	files map[string]string
}

func (p project) build(t *testing.T) (*sbom.BOM, string) {
	t.Helper()
	dir := t.TempDir()
	root := filepath.Join(dir, ".ai-rulez")
	require.NoError(t, os.MkdirAll(root, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "config.toml"), []byte(p.cfg), 0o644))
	for rel, content := range p.files {
		full := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
	}
	cfg, err := config.LoadConfig(context.Background(), dir, config.WithoutLocal())
	require.NoError(t, err)
	bom, err := sbom.Build(cfg, "9.9.9")
	require.NoError(t, err)
	var buf bytes.Buffer
	require.NoError(t, sbom.Write(&buf, bom))
	return bom, buf.String()
}

var sampleFiles = map[string]string{
	"rules/r1.md":                "# R1\n\nbody\n",
	"context/c1.md":              "context\n",
	"skills/s1/SKILL.md":         "---\nname: s1\ndescription: d\n---\nhi\n",
	"agents/a1.md":               "---\nname: a1\ndescription: d\n---\nagent\n",
	"commands/cmd1.md":           "---\ndescription: d\n---\ndo it\n",
	"checks/ck1.md":              "---\ndescription: d\nseverity: warn\n---\ncheck\n",
	"domains/backend/rules/b.md": "# B\n",
}

func schemaFor(t *testing.T) *jsonschema.Schema {
	t.Helper()
	read := func(name string) []byte {
		data, err := os.ReadFile(filepath.Join("testdata", name))
		require.NoError(t, err)
		return data
	}
	compiler := jsonschema.NewCompiler()
	compiled, err := compiler.CompileBatch(map[string][]byte{
		"http://cyclonedx.org/schema/spdx.schema.json":     read("spdx.schema.json"),
		"http://cyclonedx.org/schema/jsf-0.82.schema.json": read("jsf-0.82.schema.json"),
		"http://cyclonedx.org/schema/bom-1.6.schema.json":  read("bom-1.6.schema.json"),
	})
	require.NoError(t, err)
	return compiled["http://cyclonedx.org/schema/bom-1.6.schema.json"]
}

func TestOutputValidatesAgainstCycloneDX16(t *testing.T) {
	// Arrange
	schema := schemaFor(t)
	cases := map[string]project{
		"empty config":  {cfg: baseConfig},
		"items and mcp": {cfg: baseConfig + mcpConfig, files: sampleFiles},
		"items only":    {cfg: `version = "4.0"` + "\n" + `name = "x"` + "\n", files: sampleFiles},
	}
	for name, p := range cases {
		t.Run(name, func(t *testing.T) {
			// Act
			_, doc := p.build(t)

			// Assert
			result := schema.Validate([]byte(doc))
			if !result.IsValid() {
				details, _ := json.MarshalIndent(result.ToList(), "", " ")
				t.Fatalf("schema violations:\n%s", details)
			}
		})
	}
}

func TestEveryItemKindIsListed(t *testing.T) {
	// Arrange
	p := project{cfg: baseConfig, files: sampleFiles}

	// Act
	bom, _ := p.build(t)

	// Assert
	kinds := map[string]bool{}
	for _, c := range bom.Components {
		for _, pr := range c.Properties {
			if pr.Name == "ai-rulez:kind" {
				kinds[pr.Value] = true
			}
		}
	}
	for _, want := range []string{"rule", "context", "skill", "agent", "command", "check"} {
		assert.True(t, kinds[want], "kind %s missing from %v", want, kinds)
	}
}

func TestNoSecretIsEmitted(t *testing.T) {
	// Arrange
	p := project{cfg: baseConfig + mcpConfig, files: sampleFiles}

	// Act
	_, doc := p.build(t)

	// Assert
	for _, secret := range []string{"env-secret-value", "url-secret", "query-secret", "header-secret", "user:", "frag"} {
		assert.NotContains(t, doc, secret)
	}
	assert.Contains(t, doc, `"https://example.com"`)
	assert.Contains(t, doc, "API_KEY", "key names are listed, values are not")
}

func TestHashesFieldIsNeverUsed(t *testing.T) {
	// Arrange
	p := project{cfg: baseConfig + mcpConfig, files: sampleFiles}

	// Act
	_, doc := p.build(t)

	// Assert
	assert.NotContains(t, doc, `"hashes"`)
	assert.NotContains(t, doc, `"timestamp"`)
	assert.Contains(t, doc, `"ai-rulez:digest"`)
}

func TestOutputIsDeterministicAcrossLineEndings(t *testing.T) {
	// Arrange
	lf := project{cfg: baseConfig + mcpConfig, files: sampleFiles}
	crlf := project{cfg: baseConfig + mcpConfig, files: map[string]string{}}
	for k, v := range sampleFiles {
		crlf.files[k] = strings.ReplaceAll(v, "\n", "\r\n")
	}

	// Act
	_, first := lf.build(t)
	_, second := lf.build(t)
	_, third := crlf.build(t)

	// Assert
	assert.Equal(t, first, second, "two runs")
	assert.Equal(t, first, third, "CRLF sources")
}

func TestSerialNumberFollowsContent(t *testing.T) {
	uuidV5 := regexp.MustCompile(`^urn:uuid:[0-9a-f]{8}-[0-9a-f]{4}-5[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	tests := []struct {
		name       string
		a, b       project
		wantSameID bool
	}{
		{"same content", project{cfg: baseConfig, files: sampleFiles}, project{cfg: baseConfig, files: sampleFiles}, true},
		{"edited rule", project{cfg: baseConfig, files: sampleFiles}, project{cfg: baseConfig, files: with(sampleFiles, "rules/r1.md", "changed\n")}, false},
		{"added mcp server", project{cfg: baseConfig, files: sampleFiles}, project{cfg: baseConfig + mcpConfig, files: sampleFiles}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			a, _ := tt.a.build(t)
			b, _ := tt.b.build(t)

			// Assert
			assert.Regexp(t, uuidV5, a.SerialNumber)
			assert.Equal(t, tt.wantSameID, a.SerialNumber == b.SerialNumber)
		})
	}
}

func with(m map[string]string, k, v string) map[string]string {
	out := map[string]string{}
	for key, val := range m {
		out[key] = val
	}
	out[k] = v
	return out
}

func TestMCPServerPURLs(t *testing.T) {
	tests := []struct {
		name, command, args, wantPURL string
	}{
		{"npx scoped with version", "npx", `["-y", "@scope/pkg@1.2.3", "--flag"]`, "pkg:npm/%40scope/pkg@1.2.3"},
		{"npx unversioned", "npx", `["pkg"]`, "pkg:npm/pkg"},
		{"npx -p package", "npx", `["-p", "left-pad@2", "run"]`, "pkg:npm/left-pad@2"},
		{"npx path is not a package", "npx", `["./local.js"]`, ""},
		{"bunx", "bunx", `["pkg@3"]`, "pkg:npm/pkg@3"},
		{"pnpm dlx", "pnpm", `["dlx", "pkg@3"]`, "pkg:npm/pkg@3"},
		{"uvx pinned", "uvx", `["Mcp_Server==0.4.0"]`, "pkg:pypi/mcp-server@0.4.0"},
		{"uvx from", "uvx", `["--from", "tool@1.0", "cmd"]`, "pkg:pypi/tool@1.0"},
		{"pipx run", "pipx", `["run", "tool"]`, "pkg:pypi/tool"},
		{"docker tag", "docker", `["run", "-i", "--rm", "-e", "K=V", "ghcr.io/org/img:1.0"]`, "pkg:oci/img?repository_url=ghcr.io/org/img&tag=1.0"},
		{"docker digest", "docker", `["run", "mcp/fetch@sha256:abc"]`, "pkg:oci/fetch@sha256:abc?repository_url=docker.io/mcp/fetch"},
		{"docker userinfo is not a digest", "docker", `["run", "user:pass@registry.example/img"]`, ""},
		{"docker user@host is not a digest", "docker", `["run", "tok3n@registry.example"]`, ""},
		{"docker library image", "docker", `["run", "redis"]`, "pkg:oci/redis?repository_url=docker.io/library/redis"},
		{"docker build is not run", "docker", `["build", "."]`, ""},
		{"go run versioned", "go", `["run", "github.com/org/tool/cmd/srv@v1.2.3"]`, "pkg:golang/github.com/org/tool/cmd/srv@v1.2.3"},
		{"go run local", "go", `["run", "./cmd/srv"]`, ""},
		{"unknown launcher", "/usr/local/bin/my-server", `["--token", "abc"]`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := baseConfig + "\n[[mcp_servers]]\nname = \"srv\"\ncommand = " + quote(tt.command) + "\nargs = " + tt.args + "\n"

			// Act
			bom, doc := (project{cfg: cfg}).build(t)

			// Assert
			var srv *sbom.Component
			for i := range bom.Components {
				if bom.Components[i].BOMRef == "ai-rulez:mcp:srv" {
					srv = &bom.Components[i]
				}
			}
			require.NotNil(t, srv)
			assert.Equal(t, tt.wantPURL, srv.PURL)
			assert.NotContains(t, doc, `"args"`)
			assert.NotContains(t, doc, "--token")
			assert.NotContains(t, doc, "K=V")
		})
	}
}

func quote(s string) string { b, _ := json.Marshal(s); return string(b) }

func TestRemoteMCPServerIsAService(t *testing.T) {
	// Arrange
	p := project{cfg: baseConfig + mcpConfig}

	// Act
	bom, _ := p.build(t)

	// Assert
	require.Len(t, bom.Services, 1)
	svc := bom.Services[0]
	assert.Equal(t, "remote", svc.Name)
	assert.True(t, svc.Authenticated)
	assert.Equal(t, []string{"https://example.com"}, svc.Endpoints)
	for _, c := range bom.Components {
		assert.NotEqual(t, "ai-rulez:mcp:remote", c.BOMRef)
	}
}

func TestUnparsableEndpointIsOmitted(t *testing.T) {
	// Arrange
	p := project{cfg: baseConfig + "\n[[mcp_servers]]\nname = \"ph\"\ntransport = \"http\"\nurl = \"${SECRET_URL}\"\n"}

	// Act
	bom, doc := p.build(t)

	// Assert
	require.Len(t, bom.Services, 1)
	assert.Empty(t, bom.Services[0].Endpoints)
	assert.NotContains(t, doc, "SECRET_URL")
}

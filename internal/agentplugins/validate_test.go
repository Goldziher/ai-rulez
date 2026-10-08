package agentplugins

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mapFS is files as an in-memory file system.
func mapFS(files map[string][]byte) fstest.MapFS {
	m := fstest.MapFS{}
	for name, data := range files {
		m[name] = &fstest.MapFile{Data: data, Mode: 0o644}
	}
	return m
}

func symlink(target string) *fstest.MapFile {
	return &fstest.MapFile{Data: []byte(target), Mode: fs.ModeSymlink}
}

func builtFS(t *testing.T, spec string) fstest.MapFS {
	t.Helper()
	files, _, err := Build(fixturePlugin(), Options{Spec: spec})
	require.NoError(t, err)
	return mapFS(files)
}

func TestValidateAcceptsEveryGolden(t *testing.T) {
	for _, spec := range Specs {
		t.Run(spec, func(t *testing.T) {
			// Act
			res := Validate(os.DirFS(goldenDir(spec)))

			// Assert
			assert.False(t, res.Rejected)
			assert.Equal(t, spec, res.Spec)
			assert.Empty(t, res.Findings)
		})
	}
}

func TestEveryGoldenDocumentMatchesTheOfficialSchema(t *testing.T) {
	for _, spec := range Specs {
		set, err := schemasFor(spec)
		require.NoError(t, err)
		for _, file := range []string{"plugin.json", "mcp.json"} {
			t.Run(spec+"/"+file, func(t *testing.T) {
				// Arrange
				data, err := os.ReadFile(goldenDir(spec) + "/" + file)
				require.NoError(t, err)
				var v any
				require.NoError(t, json.Unmarshal(data, &v))
				schema := set.plugin
				if file == "mcp.json" {
					schema = set.mcp
				}

				// Act
				errs := schemaErrors(schema, v)

				// Assert
				assert.Empty(t, errs)
			})
		}
	}
}

func TestRoundTripIsByteIdentical(t *testing.T) {
	for _, spec := range Specs {
		t.Run(spec, func(t *testing.T) {
			// Arrange
			first, _, err := Build(fixturePlugin(), Options{Spec: spec})
			require.NoError(t, err)

			// Act
			model, res := Import(mapFS(first))
			require.NotNil(t, model)
			second, findings, err := Build(model, Options{Spec: res.Spec})

			// Assert
			require.NoError(t, err)
			assert.Empty(t, res.Findings)
			assert.Empty(t, findings)
			assert.Equal(t, first, second)
		})
	}
}

func TestImportOfTheGoldenDirectoryRebuildsIt(t *testing.T) {
	for _, spec := range Specs {
		t.Run(spec, func(t *testing.T) {
			model, res := Import(os.DirFS(goldenDir(spec)))
			require.NotNil(t, model)

			files, _, err := Build(model, Options{Spec: res.Spec})

			require.NoError(t, err)
			assert.Equal(t, readTree(t, goldenDir(spec)), files)
		})
	}
}

func TestImportMapsTransportsBack(t *testing.T) {
	model, _ := Import(builtFS(t, Spec100))
	require.NotNil(t, model)

	got := map[string]string{}
	for _, s := range model.MCPServers {
		got[s.Name] = s.Transport
	}

	assert.Equal(t, map[string]string{
		"deploy-api": TransportHTTP, "legacy-events": TransportSSE, "local-dev": TransportHTTP,
		"npx-server": TransportStdio, "validator": TransportStdio,
	}, got)
}

var allSkills = []string{"deploy", "summarize"}

var allServers = []string{"deploy-api", "legacy-events", "local-dev", "npx-server", "validator"}

var withOK = []string{"deploy-api", "legacy-events", "local-dev", "npx-server", "ok", "validator"}

func TestValidateAppliesTheFailureBoundaries(t *testing.T) {
	const mcp100 = `{"$schema":"https://agent-plugins.org/schemas/1.0.0/mcp.schema.json","mcpServers":`
	const plugin100 = `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json",`
	setServer := func(name, body string) func(fstest.MapFS) {
		return func(m fstest.MapFS) {
			var doc map[string]any
			if err := json.Unmarshal(m["mcp.json"].Data, &doc); err != nil {
				panic(err)
			}
			doc["mcpServers"].(map[string]any)[name] = json.RawMessage(body)
			data, err := json.Marshal(doc)
			if err != nil {
				panic(err)
			}
			m["mcp.json"].Data = data
		}
	}
	tests := []struct {
		name     string
		mutate   func(fstest.MapFS)
		rejected bool
		want     []string
		skills   []string
		servers  []string
	}{
		// plugin.json: fatal unless the problem is an unknown field or a non-object extensions.
		{name: "manifest missing", mutate: func(m fstest.MapFS) { delete(m, "plugin.json") },
			rejected: true, want: []string{"manifest-missing@plugin.json"}},
		{name: "manifest not json", mutate: func(m fstest.MapFS) { m["plugin.json"].Data = []byte("{") },
			rejected: true, want: []string{"manifest-invalid@plugin.json"}},
		{name: "manifest not an object", mutate: func(m fstest.MapFS) { m["plugin.json"].Data = []byte("[]") },
			rejected: true, want: []string{"manifest-invalid@plugin.json"}},
		{name: "manifest is a directory", mutate: func(m fstest.MapFS) {
			delete(m, "plugin.json")
			m["plugin.json/x"] = &fstest.MapFile{Data: []byte("x")}
		}, rejected: true, want: []string{"manifest-invalid@plugin.json"}},
		{name: "unknown schema version", mutate: func(m fstest.MapFS) {
			m["plugin.json"].Data = []byte(`{"$schema":"https://agent-plugins.org/schemas/9.0.0/plugin.schema.json","name":"a"}`)
		}, rejected: true, want: []string{"unsupported-spec@plugin.json"}},
		{name: "invalid plugin name", mutate: func(m fstest.MapFS) { m["plugin.json"].Data = []byte(plugin100 + `"name":"a--b"}`) },
			rejected: true, want: []string{"manifest-invalid@plugin.json"}},
		{name: "author with an extra field", mutate: func(m fstest.MapFS) {
			m["plugin.json"].Data = []byte(plugin100 + `"name":"a","author":{"name":"x","handle":"y"}}`)
		}, rejected: true, want: []string{"manifest-invalid@plugin.json"}},
		{name: "extension value not an object", mutate: func(m fstest.MapFS) {
			m["plugin.json"].Data = []byte(plugin100 + `"name":"a","extensions":{"com.example.client":true}}`)
		}, rejected: true, want: []string{"manifest-invalid@plugin.json"}},
		{name: "manifest symlink escapes", mutate: func(m fstest.MapFS) { m["plugin.json"] = symlink("../outside.json") },
			rejected: true, want: []string{"path-escape@plugin.json"}},
		{name: "unknown top-level field is ignored", mutate: func(m fstest.MapFS) {
			m["plugin.json"].Data = []byte(plugin100 + `"name":"acme.tools","skills":"./elsewhere"}`)
		}, want: []string{"manifest-unknown-field@plugin.json"}, skills: allSkills, servers: allServers},
		{name: "non-object extensions is ignored", mutate: func(m fstest.MapFS) {
			m["plugin.json"].Data = []byte(plugin100 + `"name":"acme.tools","extensions":[]}`)
		}, want: []string{"extensions-invalid@plugin.json"}, skills: allSkills, servers: allServers},
		{name: "non reverse-domain extension key", mutate: func(m fstest.MapFS) {
			m["plugin.json"].Data = []byte(plugin100 + `"name":"acme.tools","extensions":{"claude":{}}}`)
		}, want: []string{"namespace-invalid@plugin.json"}, skills: allSkills, servers: allServers},

		// skills/: a bad location disables skills; a bad skill skips that skill.
		{name: "skills is a file", mutate: func(m fstest.MapFS) {
			for k := range m {
				if len(k) > 7 && k[:7] == "skills/" {
					delete(m, k)
				}
			}
			m["skills"] = &fstest.MapFile{Data: []byte("x")}
		}, want: []string{"skills-location-invalid@skills"}, skills: []string{}, servers: allServers},
		{name: "directory without SKILL.md", mutate: func(m fstest.MapFS) { m["skills/notes/readme.md"] = &fstest.MapFile{Data: []byte("x")} },
			want: []string{"skill-md-missing@skills/notes"}, skills: allSkills, servers: allServers},
		{name: "nested skill is not discovered", mutate: func(m fstest.MapFS) {
			m["skills/group/inner/SKILL.md"] = &fstest.MapFile{Data: []byte("---\nname: inner\ndescription: d\n---\n")}
		}, want: []string{"skill-md-missing@skills/group"}, skills: allSkills, servers: allServers},
		{name: "SKILL.md is a directory", mutate: func(m fstest.MapFS) {
			delete(m, "skills/deploy/SKILL.md")
			m["skills/deploy/SKILL.md/x"] = &fstest.MapFile{Data: []byte("x")}
		}, want: []string{"skill-invalid@skills/deploy/SKILL.md"}, skills: []string{"summarize"}, servers: allServers},
		{name: "invalid skill frontmatter", mutate: func(m fstest.MapFS) {
			m["skills/deploy/SKILL.md"].Data = []byte("---\nname: other\ndescription: d\n---\n")
		},
			want: []string{"skill-invalid@skills/deploy/SKILL.md"}, skills: []string{"summarize"}, servers: allServers},
		{name: "file directly under skills is ignored", mutate: func(m fstest.MapFS) { m["skills/README.md"] = &fstest.MapFile{Data: []byte("x")} },
			skills: allSkills, servers: allServers},
		{name: "skill directory symlink inside the root", mutate: func(m fstest.MapFS) {
			m["shared/alias/SKILL.md"] = &fstest.MapFile{Data: []byte("---\nname: alias\ndescription: d\n---\n")}
			m["skills/alias"] = symlink("../shared/alias")
		}, skills: []string{"alias", "deploy", "summarize"}, servers: allServers},
		{name: "skill directory symlink escapes", mutate: func(m fstest.MapFS) { m["skills/evil"] = symlink("../../outside") },
			want: []string{"path-escape@skills/evil"}, skills: allSkills, servers: allServers},
		{name: "absolute skill symlink", mutate: func(m fstest.MapFS) { m["skills/evil"] = symlink("/etc") },
			want: []string{"path-escape@skills/evil"}, skills: allSkills, servers: allServers},
		{name: "skill directory symlink loop", mutate: func(m fstest.MapFS) { m["skills/loop"] = symlink("loop") },
			want: []string{"unreadable@skills/loop"}, skills: allSkills, servers: allServers},
		{name: "SKILL.md symlink escapes", mutate: func(m fstest.MapFS) { m["skills/deploy/SKILL.md"] = symlink("../../../outside.md") },
			want: []string{"path-escape@skills/deploy/SKILL.md"}, skills: []string{"summarize"}, servers: allServers},
		{name: "skill resource symlink escapes", mutate: func(m fstest.MapFS) { m["skills/summarize/scripts/evil.sh"] = symlink("../../../../x") },
			want: []string{"path-escape@skills/summarize/scripts/evil.sh"}, skills: allSkills, servers: allServers},
		{name: "root file symlink escapes", mutate: func(m fstest.MapFS) { m["NOTICE"] = symlink("../NOTICE") },
			want: []string{"path-escape@NOTICE"}, skills: allSkills, servers: allServers},

		// mcp.json: a bad document disables MCP; a bad entry skips that server.
		{name: "mcp.json not json", mutate: func(m fstest.MapFS) { m["mcp.json"].Data = []byte("{") },
			want: []string{"mcp-invalid@mcp.json"}, skills: allSkills, servers: []string{}},
		{name: "mcp.json is a directory", mutate: func(m fstest.MapFS) {
			delete(m, "mcp.json")
			m["mcp.json/x"] = &fstest.MapFile{Data: []byte("x")}
		}, want: []string{"mcp-invalid@mcp.json"}, skills: allSkills, servers: []string{}},
		{name: "mcp.json targets another version", mutate: func(m fstest.MapFS) {
			m["mcp.json"].Data = []byte(`{"$schema":"https://agent-plugins.org/schemas/1.1.0/mcp.schema.json","mcpServers":{}}`)
		}, want: []string{"mcp-spec-mismatch@mcp.json"}, skills: allSkills, servers: []string{}},
		{name: "mcp.json unknown version", mutate: func(m fstest.MapFS) {
			m["mcp.json"].Data = []byte(`{"$schema":"https://example.com/mcp.json","mcpServers":{}}`)
		}, want: []string{"mcp-invalid@mcp.json"}, skills: allSkills, servers: []string{}},
		{name: "mcp.json extra top-level field", mutate: func(m fstest.MapFS) { m["mcp.json"].Data = []byte(mcp100 + `{},"servers":{}}`) },
			want: []string{"mcp-invalid@mcp.json"}, skills: allSkills, servers: []string{}},
		{name: "empty mcpServers", mutate: func(m fstest.MapFS) { m["mcp.json"].Data = []byte(mcp100 + `{}}`) },
			skills: allSkills, servers: []string{}},
		{name: "server with a field of another variant", mutate: setServer("bad", `{"type":"stdio","command":"x","url":"https://x.example"}`),
			want: []string{"mcp-server-invalid@mcp.json#/mcpServers/bad"}, skills: allSkills, servers: allServers},
		{name: "server with an unknown type", mutate: setServer("bad", `{"type":"websocket","url":"wss://x.example"}`),
			want: []string{"mcp-server-invalid@mcp.json#/mcpServers/bad"}, skills: allSkills, servers: allServers},
		{name: "shell command string", mutate: setServer("bad", `{"type":"stdio","command":"npx -y srv"}`),
			want: []string{"mcp-server-invalid@mcp.json#/mcpServers/bad"}, skills: allSkills, servers: allServers},
		{name: "reserved env key", mutate: setServer("bad", `{"type":"stdio","command":"x","env":{"PLUGIN_DATA":"/x"}}`),
			want: []string{"mcp-server-invalid@mcp.json#/mcpServers/bad"}, skills: allSkills, servers: allServers},
		{name: "cwd escapes", mutate: setServer("bad", `{"type":"stdio","command":"x","cwd":"${PLUGIN_DATA}/../x"}`),
			want: []string{"mcp-server-invalid@mcp.json#/mcpServers/bad"}, skills: allSkills, servers: allServers},
		{name: "http to a remote host", mutate: setServer("bad", `{"type":"streamable-http","url":"http://x.example/mcp"}`),
			want: []string{"mcp-server-invalid@mcp.json#/mcpServers/bad"}, skills: allSkills, servers: allServers},
		{name: "bundled command symlink escapes", mutate: func(m fstest.MapFS) { m["bin/validator"] = symlink("../../bin/sh") },
			want:   []string{"path-escape@bin/validator", "mcp-server-invalid@mcp.json#/mcpServers/validator"},
			skills: allSkills, servers: []string{"deploy-api", "legacy-events", "local-dev", "npx-server"}},
		{name: "foreign placeholder is literal", mutate: setServer("ok", `{"type":"stdio","command":"x","args":["${TOKEN}"]}`),
			want: []string{"placeholder-unsupported@mcp.json#/mcpServers/ok"}, skills: allSkills, servers: withOK},
		{name: "credential header", mutate: setServer("ok", `{"type":"streamable-http","url":"https://x.example","headers":{"Authorization":"Bearer x"}}`),
			want: []string{"credential-header@mcp.json#/mcpServers/ok"}, skills: allSkills, servers: withOK},
		{name: "bundled command missing", mutate: func(m fstest.MapFS) { delete(m, "bin/validator") },
			want: []string{"command-not-bundled@mcp.json#/mcpServers/validator"}, skills: allSkills, servers: allServers},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			m := builtFS(t, Spec100)
			tt.mutate(m)

			// Act
			res := Validate(m)
			model, imported := Import(m)

			// Assert
			assert.Equal(t, tt.rejected, res.Rejected)
			assert.Equal(t, tt.want, nilIfEmpty(codes(res.Findings)), "%+v", res.Findings)
			assert.Equal(t, res, imported, "Validate reports what Import reports")
			if tt.rejected {
				assert.Nil(t, model)
				return
			}
			require.NotNil(t, model)
			gotSkills := []string{}
			for _, s := range model.Skills {
				gotSkills = append(gotSkills, s.Name)
			}
			gotServers := []string{}
			for _, s := range model.MCPServers {
				gotServers = append(gotServers, s.Name)
			}
			assert.Equal(t, tt.skills, gotSkills)
			assert.Equal(t, tt.servers, gotServers)
		})
	}
}

func TestImportReadsResolvedSymlinks(t *testing.T) {
	// Arrange
	m := builtFS(t, Spec100)
	m["shared/analyze.sh"] = &fstest.MapFile{Data: []byte("#!/bin/sh\n")}
	m["skills/summarize/scripts/linked.sh"] = symlink("../../../shared/analyze.sh")

	// Act
	model, res := Import(m)

	// Assert
	require.NotNil(t, model)
	assert.Empty(t, res.Findings)
	var summarize Skill
	for _, s := range model.Skills {
		if s.Name == "summarize" {
			summarize = s
		}
	}
	assert.Equal(t, "#!/bin/sh\n", string(summarize.Files["scripts/linked.sh"]))
	assert.Equal(t, "#!/bin/sh\n", string(model.Files["shared/analyze.sh"]))
}

func TestImportPreservesExtensionManifestNumbers(t *testing.T) {
	// Arrange
	m := builtFS(t, Spec100)
	m["plugin.json"].Data = []byte(`{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"a",` +
		`"extensions":{"com.example.client":{"ratio":1.50,"big":12345678901234567890}}}`)

	// Act
	model, _ := Import(m)
	require.NotNil(t, model)
	files, _, err := Build(model, Options{})

	// Assert
	require.NoError(t, err)
	assert.Contains(t, string(files["plugin.json"]), `"ratio": 1.50`)
	assert.Contains(t, string(files["plugin.json"]), `"big": 12345678901234567890`)
}

func TestValidateOnRealSymlinks(t *testing.T) {
	// Arrange
	root := t.TempDir()
	plugin := filepath.Join(root, "plugin")
	files, _, err := Build(fixturePlugin(), Options{})
	require.NoError(t, err)
	writeTree(t, plugin, files)
	require.NoError(t, os.WriteFile(filepath.Join(root, "secret.txt"), []byte("s"), 0o600))
	links := map[string]string{
		"skills/escape":                      filepath.Join("..", ".."),
		"skills/summarize/references/abs.md": filepath.Join(root, "secret.txt"),
		"skills/summarize/references/rel.md": filepath.Join("..", "..", "..", "..", "secret.txt"),
		"skills/summarize/references/ok.md":  "checklist.md",
	}
	for name, target := range links {
		testutil.SymlinkOrSkip(t, target, filepath.Join(plugin, filepath.FromSlash(name)))
	}

	// Act
	model, res := Import(os.DirFS(plugin))

	// Assert
	require.NotNil(t, model)
	assert.Equal(t, []string{
		"path-escape@skills/escape",
		"path-escape@skills/summarize/references/abs.md",
		"path-escape@skills/summarize/references/rel.md",
	}, codes(res.Findings))
	for _, s := range model.Skills {
		if s.Name == "summarize" {
			assert.Equal(t, "- scope\n- risk\n", string(s.Files["references/ok.md"]))
			assert.NotContains(t, s.Files, "references/abs.md")
		}
	}
}

func TestImportSkipsOversizeFiles(t *testing.T) {
	// Arrange
	plugin := filepath.Join(t.TempDir(), "plugin")
	files, _, err := Build(fixturePlugin(), Options{})
	require.NoError(t, err)
	writeTree(t, plugin, files)
	big, err := os.Create(filepath.Join(plugin, "skills", "summarize", "references", "big.bin"))
	require.NoError(t, err)
	require.NoError(t, big.Truncate(maxFileBytes+1)) // sparse: no disk or memory cost
	require.NoError(t, big.Close())

	// Act
	model, res := Import(os.DirFS(plugin))

	// Assert
	require.NotNil(t, model)
	assert.Contains(t, codes(res.Findings), "file-too-large@skills/summarize/references/big.bin")
	for _, s := range model.Skills {
		assert.NotContains(t, s.Files, "references/big.bin")
	}
}

func TestImportStopsAtTheTotalCap(t *testing.T) {
	// Arrange
	old := maxTotalBytes
	maxTotalBytes = 1 << 10
	t.Cleanup(func() { maxTotalBytes = old })
	plugin := filepath.Join(t.TempDir(), "plugin")
	files, _, err := Build(fixturePlugin(), Options{})
	require.NoError(t, err)
	writeTree(t, plugin, files)
	extra := filepath.Join(plugin, "skills", "summarize", "references")
	for _, n := range []string{"a.bin", "b.bin"} {
		require.NoError(t, os.WriteFile(filepath.Join(extra, n), make([]byte, 800), 0o600))
	}

	// Act
	_, res := Import(os.DirFS(plugin))

	// Assert
	var found bool
	for _, f := range res.Findings {
		found = found || f.Code == CodeFileTooLarge && strings.Contains(f.Message, "total")
	}
	assert.True(t, found, "findings: %v", codes(res.Findings))
}

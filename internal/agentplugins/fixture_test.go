package agentplugins

import (
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

// fixturePlugin is the golden plugin: skills with resources, stdio and remote
// MCP servers, extension namespaces with manifest data and files, and bundled
// root files.
func fixturePlugin() *Plugin {
	return &Plugin{
		Metadata: Metadata{
			Name:        "acme.tools",
			Version:     "1.2.0",
			Description: "Acme review tooling <beta> & checks.",
			Author:      &Author{Name: "Acme", Email: "dev@acme.example", URL: "https://acme.example"},
			Homepage:    "https://acme.example/tools",
			Repository:  "https://github.com/acme/tools",
			License:     "MIT",
			Keywords:    []string{"review", "mcp"},
		},
		Skills: []Skill{
			{
				Name:    "summarize",
				SkillMD: []byte("---\nname: summarize\ndescription: Summarize a diff for review.\nlicense: MIT\nmetadata:\n  owner: acme\n---\n\n# Summarize\n\nRun `scripts/analyze.sh`.\n"),
				Files: map[string][]byte{
					"scripts/analyze.sh":      []byte("#!/bin/sh\necho analyzed\n"),
					"references/checklist.md": []byte("- scope\n- risk\n"),
				},
			},
			{
				Name:    "deploy",
				SkillMD: []byte("---\nname: deploy\ndescription: Deploy the service <safely> & roll back on failure.\n---\n\nDeploy.\n"),
			},
		},
		MCPServers: []MCPServer{
			{
				Name:    "validator",
				Command: "${PLUGIN_ROOT}/bin/validator",
				Args:    []string{"--data", "${PLUGIN_DATA}/validator"},
				Env:     map[string]string{"CONFIG": "${PLUGIN_ROOT}/config.json", "GITHUB_TOKEN": "${GITHUB_TOKEN}"},
				Cwd:     "${PLUGIN_ROOT}",
			},
			{Name: "npx-server", Command: "npx", Args: []string{"-y", "@acme/mcp"}},
			{Name: "deploy-api", Transport: "http", URL: "https://deploy.acme.example/mcp", Headers: map[string]string{"X-Tenant": "public"}},
			{Name: "local-dev", Transport: "http", URL: "http://localhost:8787/mcp"},
			{Name: "legacy-events", Transport: "sse", URL: "https://legacy.acme.example/sse"},
		},
		Extensions: []Extension{
			{
				Namespace: NamespaceClaudeCode,
				Files: map[string][]byte{
					"agents/reviewer.md": []byte("---\nname: reviewer\ndescription: Reviews diffs.\n---\n\nReview.\n"),
					"commands/review.md": []byte("---\ndescription: Review the diff.\n---\n\nReview $ARGUMENTS.\n"),
					"hooks/hooks.json":   []byte("{\n  \"hooks\": {}\n}\n"),
				},
			},
			{
				Namespace: "com.example.client",
				Manifest:  map[string]any{"setting": true, "level": 2, "nested": map[string]any{"b": "x", "a": []any{"y"}}},
			},
		},
		Files: map[string][]byte{
			"LICENSE":       []byte("MIT License\n"),
			"bin/validator": []byte("#!/bin/sh\nexec node \"$PLUGIN_ROOT/server.js\"\n"),
		},
	}
}

// readTree reads every regular file under dir into a slash-path map.
func readTree(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(p) //nolint:gosec // test fixture path
		out[filepath.ToSlash(rel)] = data
		return err
	})
	require.NoError(t, err)
	return out
}

// writeTree replaces dir with files.
func writeTree(t *testing.T, dir string, files map[string][]byte) {
	t.Helper()
	require.NoError(t, os.RemoveAll(dir))
	for _, name := range slices.Sorted(maps.Keys(files)) {
		target := filepath.Join(dir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o755))
		require.NoError(t, os.WriteFile(target, files[name], 0o644)) //nolint:gosec // test fixture
	}
}

func goldenDir(spec string) string { return path.Join("testdata", "golden", spec) }

func codes(findings []Finding) []string {
	out := make([]string, 0, len(findings))
	for _, f := range findings {
		out = append(out, f.Code+"@"+f.Path)
	}
	return out
}

package opencodev1

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsV1Plugin(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   bool
	}{
		{
			name:   "v1 typed named async export",
			source: "import type { Plugin } from \"@opencode-ai/plugin\"\nexport const Example: Plugin = async ({ client }) => ({})\n",
			want:   true,
		},
		{
			name:   "v1 untyped named async arrow export",
			source: "export const Example = async (ctx) => {\n  return {}\n}\n",
			want:   true,
		},
		{
			name:   "v1 named async function export",
			source: "export async function Example({ directory }) {\n  return {}\n}\n",
			want:   true,
		},
		{
			name:   "v1 default async arrow",
			source: "export default async (ctx) => ({})\n",
			want:   true,
		},
		{
			name:   "v1 default async function",
			source: "export default async function plugin(ctx) { return {} }\n",
			want:   true,
		},
		{
			name:   "v1 default identifier arrow",
			source: "export default ctx => ({})\n",
			want:   true,
		},
		{
			name:   "v1 package import without id",
			source: "import { tool } from \"@opencode-ai/plugin\"\nexport const X = async () => ({ tool: {} })\n",
			want:   true,
		},
		{
			name:   "v2 define",
			source: "import { Plugin } from \"@opencode/plugin\"\nexport default Plugin.define({ id: \"x\", async setup(ctx) {} })\n",
			want:   false,
		},
		{
			name:   "v2 plain object",
			source: "export default {\n  id: \"x\",\n  async setup(ctx) {},\n}\n",
			want:   false,
		},
		{
			name:   "v2 effect plugin",
			source: "export default { id: \"x\", effect: (ctx) => Effect.void }\n",
			want:   false,
		},
		{
			name:   "dual v1 and v2 object",
			source: "import { Plugin } from \"@opencode/plugin\"\nexport default {\n  ...Plugin.define({ id: \"x\", async setup() {} }),\n  async server() { return {} },\n}\n",
			want:   false,
		},
		{
			name:   "v2 file with async helper export",
			source: "export async function helper() {}\nexport default { id: \"x\", setup: async (ctx) => { await helper() } }\n",
			want:   false,
		},
		{
			name:   "commented v1 example in a v2 file",
			source: "// export default async (ctx) => ({})\nexport default { id: \"x\", setup() {} }\n",
			want:   false,
		},
		{
			name:   "v1 in a block comment only",
			source: "/* export const X = async () => ({}) */\nexport default { id: \"x\", setup() {} }\n",
			want:   false,
		},
		{
			name:   "empty file",
			source: "",
			want:   false,
		},
		{
			name:   "plain module with no plugin export",
			source: "export const VALUE = 1\nexport function sync() {}\n",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got := IsV1Plugin(tt.source)

			// Assert
			assert.Equal(t, tt.want, got)
		})
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

const (
	v1Source = "export const P = async () => ({})\n"
	v2Source = "export default { id: \"p\", async setup() {} }\n"
)

func TestScanProject(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  []string
	}{
		{
			name:  "no opencode files",
			files: map[string]string{},
			want:  nil,
		},
		{
			name: "plugin and plugins directories",
			files: map[string]string{
				".opencode/plugin/old.js":    v1Source,
				".opencode/plugins/old.ts":   v1Source,
				".opencode/plugins/old.mjs":  v1Source,
				".opencode/plugins/new.js":   v2Source,
				".opencode/plugins/note.txt": v1Source,
			},
			want: []string{".opencode/plugin/old.js", ".opencode/plugins/old.mjs", ".opencode/plugins/old.ts"},
		},
		{
			name: "directory plugin entry",
			files: map[string]string{
				".opencode/plugins/dir/index.ts": v1Source,
				".opencode/plugins/ok/index.ts":  v2Source,
			},
			want: []string{".opencode/plugins/dir/index.ts"},
		},
		{
			name: "opencode.json local plugin paths",
			files: map[string]string{
				"opencode.json": `{"plugin": ["./local/v1.js", "./local/v2.js", "some-npm-package", ["./local/opts.js", {"a": 1}]]}`,
				"local/v1.js":   v1Source,
				"local/v2.js":   v2Source,
				"local/opts.js": v1Source,
			},
			want: []string{"local/opts.js", "local/v1.js"},
		},
		{
			name: "opencode.jsonc with comments and v2 key",
			files: map[string]string{
				".opencode/opencode.jsonc": "{\n  // comment\n  \"plugins\": [\n    {\"package\": \"./p/v1.ts\"}, /* inline */\n  ],\n}\n",
				".opencode/p/v1.ts":        v1Source,
			},
			want: []string{".opencode/p/v1.ts"},
		},
		{
			name: "config plugin directory with index file",
			files: map[string]string{
				"opencode.json": `{"plugin": ["./pkg"]}`,
				"pkg/index.js":  v1Source,
			},
			want: []string{"pkg/index.js"},
		},
		{
			name: "same file referenced twice is reported once",
			files: map[string]string{
				"opencode.json":            `{"plugin": ["./.opencode/plugins/old.js"]}`,
				".opencode/plugins/old.js": v1Source,
			},
			want: []string{".opencode/plugins/old.js"},
		},
		{
			name: "invalid json and missing files are ignored",
			files: map[string]string{
				"opencode.json": `{"plugin": ["./missing.js", "../outside-escape.js"`,
			},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			for rel, content := range tt.files {
				writeFile(t, filepath.Join(root, filepath.FromSlash(rel)), content)
			}

			// Act
			got := ScanProject(root)

			// Assert
			var rels []string
			for _, finding := range got {
				rel, err := filepath.Rel(root, finding.Path)
				require.NoError(t, err)
				rels = append(rels, filepath.ToSlash(rel))
			}
			assert.Equal(t, tt.want, rels)
		})
	}
}

func TestWarnProjectWarnsOncePerFile(t *testing.T) {
	// Arrange
	ResetWarned()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".opencode", "plugins", "old.js"), v1Source)

	// Act
	first := WarnProject(root)
	second := WarnProject(root)

	// Assert
	assert.Len(t, first, 1)
	assert.Empty(t, second, "a file already warned about in this run is not reported again")
}

func TestMigrationHintNamesDocs(t *testing.T) {
	assert.Contains(t, MigrationHint, "https://opencode.ai/v2/docs/build/plugins/migrate-v1")
	assert.Contains(t, MigrationHint, "Plugin.define")
}

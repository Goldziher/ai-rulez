package importer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const bomPrefix = "\xef\xbb\xbf"

// TestJSONInputs_ToleranceOfAUTF8BOM pins RV-CLI-4: a JSON or JSONC input saved
// with a byte order mark is read like the same file without one, as Markdown
// inputs already are.
func TestJSONInputs_ToleranceOfAUTF8BOM(t *testing.T) {
	tests := []struct {
		name     string
		importer Format
		files    map[string]string
		check    func(t *testing.T, p *Plan)
	}{
		{
			name:     "claude settings",
			importer: nativeImporter{},
			files: map[string]string{
				"CLAUDE.md":             "x\n",
				".claude/settings.json": bomPrefix + `{"permissions":{"deny":["Read(.env)"]},"hooks":{"Stop":[{"hooks":[{"type":"command","command":"echo hi"}]}]}}`,
			},
			check: func(t *testing.T, p *Plan) {
				assert.Equal(t, []string{"Read(.env)"}, p.Permissions.Deny)
				assert.Len(t, p.Hooks, 1)
			},
		},
		{
			name:     "cursor cli permissions",
			importer: nativeImporter{},
			files:    map[string]string{"CLAUDE.md": "x\n", ".cursor/cli.json": bomPrefix + `{"permissions":{"allow":[],"deny":["Shell(rm)"]}}`},
			check: func(t *testing.T, p *Plan) {
				assert.Equal(t, []string{"Bash(rm:*)"}, p.Permissions.Deny)
			},
		},
		{
			name:     "mcp file",
			importer: nativeImporter{},
			files:    map[string]string{"CLAUDE.md": "x\n", ".mcp.json": bomPrefix + `{"mcpServers":{"fs":{"command":"npx","args":["fs"]}}}`},
			check: func(t *testing.T, p *Plan) {
				require.Len(t, p.MCPServers, 1)
				assert.Equal(t, "fs", p.MCPServers[0].Name)
			},
		},
		{
			name:     "skills lock",
			importer: skillsLockImporter{},
			files:    map[string]string{"skills-lock.json": bomPrefix + lockV1},
			check: func(t *testing.T, p *Plan) {
				assert.NotEmpty(t, p.InstalledSkills)
			},
		},
		{
			name:     "rulesync config and permissions",
			importer: rulesyncImporter{},
			files: map[string]string{
				"rulesync.jsonc":              bomPrefix + `{"targets":["claudecode"]}`,
				".rulesync/rules/a.md":        "---\nroot: true\n---\nbody\n",
				".rulesync/permissions.jsonc": bomPrefix + `{"permission":{"read":{".env":"deny"}}}`,
			},
			check: func(t *testing.T, p *Plan) {
				assert.Equal(t, []string{"Read(.env)"}, p.Permissions.Deny)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			p := planOf(t, tt.importer, mapFS(tt.files), Options{})

			// Assert
			tt.check(t, p)
			for _, f := range p.Findings {
				assert.NotContains(t, f.Reason, "invalid character", "%s: %s", f.Source, f.Reason)
			}
		})
	}
}

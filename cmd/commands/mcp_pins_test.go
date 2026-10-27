package commands

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/mcp"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

// pinSession serves the authoring MCP server of the command line, with its real pin engine.
func pinSession(t *testing.T, opts ...mcp.Option) *sdkmcp.ClientSession {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)
	srv := NewAuthoringMCPServer(append([]mcp.Option{mcp.WithAnyDirectory()}, opts...)...)
	serverT, clientT := sdkmcp.NewInMemoryTransports()
	go func() { _ = srv.GetMCPServer().Run(ctx, serverT) }()
	session, err := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "pins", Version: "1"}, nil).Connect(ctx, clientT, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func callPin(t *testing.T, s *sdkmcp.ClientSession, tool, dir string, args map[string]any) *sdkmcp.CallToolResult {
	t.Helper()
	in := map[string]any{"working_directory": dir}
	for k, v := range args {
		in[k] = v
	}
	res, err := s.CallTool(context.Background(), &sdkmcp.CallToolParams{Name: tool, Arguments: in})
	require.NoError(t, err)
	return res
}

func resultText(t *testing.T, res *sdkmcp.CallToolResult, i int) string {
	t.Helper()
	require.Greater(t, len(res.Content), i)
	text, ok := res.Content[i].(*sdkmcp.TextContent)
	require.True(t, ok)
	return text.Text
}

// structured decodes the structuredContent of a result.
func structured(t *testing.T, res *sdkmcp.CallToolResult) map[string]any {
	t.Helper()
	data, err := json.Marshal(res.StructuredContent)
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(data, &doc), string(data))
	return doc
}

func TestMCPLockOutdatedMatchesCLIJSON(t *testing.T) {
	// Arrange: v1.2.0 is allowed by the constraint and v2.0.0 is a newer major.
	f := newUpdateFixture(t, `version = "^1"`)
	f.release("one point two", "v1.2.0", false)
	lockOutdated, lockFormat = true, formatJSON
	var code int
	want := compactJSON(t, captureStdout(t, func() { code = reported(outdatedAt("", "", nil)) }))
	require.Equal(t, 0, code)
	session := pinSession(t)

	// Act
	res := callPin(t, session, "lock_outdated", f.root, nil)

	// Assert
	require.False(t, res.IsError, resultText(t, res, 0))
	assert.Equal(t, want, resultText(t, res, 0))
	validateAgainst(t, "../../schema/lock-outdated.schema.json", []byte(resultText(t, res, 0)))
	assert.Equal(t, "v1.1.0", f.lock().Find(lockfile.KindInclude, "shared").Tag, "lock_outdated writes nothing")
}

func TestMCPLockOutdatedUnknownNameIsAToolError(t *testing.T) {
	f := newUpdateFixture(t, `version = "^1"`)
	session := pinSession(t)

	res := callPin(t, session, "lock_outdated", f.root, map[string]any{"names": []string{"nope"}})

	require.True(t, res.IsError)
	assert.Contains(t, resultText(t, res, 0), "not a remote include")
}

func TestMCPUpdatePlanMatchesCLIDryRunAndWritesNothing(t *testing.T) {
	// Arrange
	f := newUpdateFixture(t, `version = "^1"`)
	f.release("one point two", "v1.2.0", false)
	before := f.lock()
	updateDryRun, updateFormat = true, formatJSON
	want := compactJSON(t, captureStdout(t, func() { require.Equal(t, 0, reported(runUpdate(nil))) }))
	resetUpdateFlags(t)
	session := pinSession(t)

	// Act
	res := callPin(t, session, "update_plan", f.root, nil)

	// Assert
	require.False(t, res.IsError, resultText(t, res, 0))
	assert.Equal(t, want, resultText(t, res, 0))
	validateAgainst(t, "../../schema/update.schema.json", []byte(resultText(t, res, 0)))
	assert.Equal(t, true, structured(t, res)["dry_run"])
	assert.Equal(t, before, f.lock(), "update_plan writes nothing")
}

func TestMCPUpdateMovesThePinAndNamesTheNextStep(t *testing.T) {
	// Arrange
	f := newUpdateFixture(t, `version = "^1"`)
	f.release("one point two", "v1.2.0", false)
	session := pinSession(t)

	// Act
	res := callPin(t, session, "update", f.root, map[string]any{"names": []string{"shared"}, "kind": "include"})

	// Assert
	require.False(t, res.IsError, resultText(t, res, 0))
	validateAgainst(t, "../../schema/update.schema.json", []byte(resultText(t, res, 0)))
	assert.Equal(t, "v1.2.0", f.lock().Find(lockfile.KindInclude, "shared").Tag)
	assert.Contains(t, resultText(t, res, 1), "ai-rulez generate")
	assert.Equal(t, false, structured(t, res)["dry_run"])

	// A second call has nothing left to move.
	again := callPin(t, session, "update", f.root, nil)
	require.False(t, again.IsError, resultText(t, again, 0))
	assert.Empty(t, structured(t, again)["updates"])
}

func TestMCPUpdateRefusalsAreStructuredToolErrors(t *testing.T) {
	tests := []struct {
		name     string
		arrange  func(t *testing.T, f *updateFixture)
		wantCode string
	}{
		{"a moved tag", func(t *testing.T, f *updateFixture) { f.release("evil", "v1.1.0", true) }, "AR732"},
		{"an unsatisfiable constraint", func(t *testing.T, f *updateFixture) { f.writeVersion(t, `version = "^7"`) }, "AR730"},
	}
	for _, tt := range tests {
		for _, tool := range []string{"update", "update_plan"} {
			t.Run(tt.name+"/"+tool, func(t *testing.T) {
				// Arrange
				f := newUpdateFixture(t, `version = "^1"`)
				before := f.lock()
				tt.arrange(t, f)
				session := pinSession(t)

				// Act
				res := callPin(t, session, tool, f.root, nil)

				// Assert
				require.True(t, res.IsError)
				assert.Contains(t, resultText(t, res, 0), tt.wantCode)
				assert.Contains(t, resultText(t, res, 1), "nothing was written")
				blocked, ok := structured(t, res)["blocked"].([]any)
				require.True(t, ok, "the refused sources are in structuredContent")
				require.Len(t, blocked, 1)
				assert.Equal(t, tt.wantCode, blocked[0].(map[string]any)["code"])
				assert.Equal(t, before, f.lock(), "a refusal writes nothing")
			})
		}
	}
}

func TestMCPUpdateScanFindingsRefuseThePin(t *testing.T) {
	// Arrange: v1.3.0 adds a file with a secret-shaped string.
	f := newAgeFixture(t, "", `version = "^1"`)
	require.Equal(t, 0, writeLockAt("", "", nil))
	before := f.lock()
	f.repo.Write(".ai-rulez/rules/leak.md", "# Leak\naws_secret_access_key = \"wJalrXUtnFEMI/K7MDENG/bPxRfiCYQ9d3kGh7Lz\"\nAKIAQYLPMN5HHHFPZAM2\n")
	f.release("one point three", "v1.3.0", false)
	session := pinSession(t)

	for _, tool := range []string{"update_plan", "update"} {
		t.Run(tool, func(t *testing.T) {
			// Act
			res := callPin(t, session, tool, f.root, nil)

			// Assert
			require.True(t, res.IsError, resultText(t, res, 0))
			updates, ok := structured(t, res)["updates"].([]any)
			require.True(t, ok)
			require.Len(t, updates, 1)
			scan := updates[0].(map[string]any)["scan"].(map[string]any)
			assert.Equal(t, true, scan["refused"])
			assert.Equal(t, before, f.lock(), "a refused scan writes nothing")
		})
	}
}

func TestMCPPinToolsAreListedByReadOnlyServersAsDocumented(t *testing.T) {
	testutil.IsolateHome(t)
	tests := []struct {
		name string
		opts []mcp.Option
		want map[string]bool
	}{
		{"read-only", []mcp.Option{mcp.WithReadOnly()}, map[string]bool{"lock_outdated": true, "update_plan": true, "lock_diff": true, "update": false}},
		{"writable", nil, map[string]bool{"lock_outdated": true, "update_plan": true, "lock_diff": true, "update": true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session := pinSession(t, tt.opts...)
			listed := map[string]bool{}
			for tool, err := range session.Tools(context.Background(), nil) {
				require.NoError(t, err)
				listed[tool.Name] = true
			}
			for name, want := range tt.want {
				assert.Equal(t, want, listed[name], name)
			}
		})
	}
}

func TestMCPLockDiffMatchesCLIJSON(t *testing.T) {
	// Arrange: a source changed after pinning.
	testutil.IsolateHome(t)
	resetLockViewFlags(t)
	root := lockProject(t, "")
	require.Equal(t, 0, writeLockAt("", "", nil))
	writeFile(t, root+"/.ai-rulez/rules/style.md", "# Style\nchanged\n")
	lockDiffFlag, lockFormat = true, formatJSON
	var code int
	want := compactJSON(t, captureStdout(t, func() { code = diffLockAt("") }))
	require.Equal(t, 0, code)
	session := pinSession(t)

	// Act
	res := callPin(t, session, "lock_diff", root, nil)

	// Assert
	require.False(t, res.IsError, resultText(t, res, 0))
	assert.Equal(t, want, resultText(t, res, 0))
	assert.Contains(t, want, `"in_sync":false`)
	validateAgainst(t, "../../schema/lock-diff.schema.json", []byte(resultText(t, res, 0)))
}

package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const governanceConfig = `version = "5.0"
name = "gov"
presets = ["claude"]

[[roles]]
name = "base"
description = "Everyone"
[roles.skills]
exclude = ["deploy"]

[[roles]]
name = "ops"
extends = "base"
`

func governanceProject(t *testing.T, extraConfig string) string {
	t.Helper()
	dir := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(dir, ".ai-rulez", filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(content), 0o600))
	}
	write("config.toml", governanceConfig+extraConfig)
	for _, id := range []string{"alpha", "beta", "gamma"} {
		write("rules/"+id+".md", "# "+id+"\nBe "+id+".\n")
	}
	write("skills/deploy/SKILL.md", "---\nname: deploy\ndescription: Deploy the service. Use when releasing.\n---\nDeploy.\n")
	write("skills/review/SKILL.md", "---\nname: review\ndescription: Review code. Use when reviewing.\n---\nReview.\n")
	return dir
}

func callGov(t *testing.T, h func(context.Context, *ToolRequest) (*sdkmcp.CallToolResult, error), dir string, args map[string]any) (*sdkmcp.CallToolResult, map[string]any) {
	t.Helper()
	in := map[string]any{"working_directory": dir}
	for k, v := range args {
		in[k] = v
	}
	res, err := h(t.Context(), NewToolRequest(nil, in))
	require.NoError(t, err)
	var doc map[string]any
	if !res.IsError {
		require.NoError(t, json.Unmarshal([]byte(textOf(t, res)), &doc), textOf(t, res))
	}
	return res, doc
}

func TestCatalogHandler_FiltersAndCaps(t *testing.T) {
	dir := governanceProject(t, "")
	tests := []struct {
		name          string
		args          map[string]any
		wantItems     int
		wantTruncated bool
		wantTotal     float64
		wantErr       string
	}{
		{"all items", nil, 5, false, 0, ""},
		{"kind filter", map[string]any{"kind": "rule"}, 3, false, 0, ""},
		{"role filter drops what the role excludes", map[string]any{"role": "ops", "kind": "skill"}, 1, false, 0, ""},
		{"limit cuts the list and says so", map[string]any{"limit": float64(2)}, 2, true, 5, ""},
		{"unknown kind", map[string]any{"kind": "nope"}, 0, false, 0, "unknown kind"},
		{"unknown role", map[string]any{"role": "ghost"}, 0, false, 0, "not defined"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			res, doc := callGov(t, CatalogHandler("test"), dir, tt.args)

			// Assert
			if tt.wantErr != "" {
				require.True(t, res.IsError)
				assert.Contains(t, textOf(t, res), tt.wantErr)
				return
			}
			require.False(t, res.IsError, textOf(t, res))
			assert.Len(t, doc["items"], tt.wantItems)
			assert.Equal(t, tt.wantTruncated, doc["truncated"] == true)
			if tt.wantTruncated {
				assert.Equal(t, tt.wantTotal, doc["total_items"])
			}
		})
	}
}

func TestResolveRoleHandler(t *testing.T) {
	dir := governanceProject(t, "")

	t.Run("resolves and caps the item list", func(t *testing.T) {
		// Act
		res, doc := callGov(t, ResolveRoleHandler, dir, map[string]any{"role": "ops", "limit": float64(1)})

		// Assert
		require.False(t, res.IsError, textOf(t, res))
		role := doc["role"].(map[string]any)
		assert.Len(t, role["items"], 1)
		assert.Equal(t, true, doc["truncated"])
		assert.Equal(t, float64(4), doc["total_items"])
		assert.Equal(t, float64(4), role["totals"].(map[string]any)["items"], "totals count every item")
	})
	t.Run("role is required", func(t *testing.T) {
		res, _ := callGov(t, ResolveRoleHandler, dir, nil)
		require.True(t, res.IsError)
	})
	t.Run("unknown role is a tool error", func(t *testing.T) {
		res, _ := callGov(t, ResolveRoleHandler, dir, map[string]any{"role": "ghost"})
		require.True(t, res.IsError)
	})
}

func TestListRolesHandler(t *testing.T) {
	_, doc := callGov(t, ListRolesHandler, governanceProject(t, ""), nil)

	assert.Len(t, doc["roles"], 2)
}

func TestLockStatusHandler(t *testing.T) {
	dir := governanceProject(t, "")
	h := LockStatusHandler("test", nil)

	t.Run("no lock is in sync unless enforced", func(t *testing.T) {
		res, doc := callGov(t, h, dir, nil)
		require.False(t, res.IsError, textOf(t, res))
		assert.Equal(t, true, doc["in_sync"])
		assert.Equal(t, []any{}, doc["changes"])
	})
	t.Run("unknown kind is a tool error", func(t *testing.T) {
		res, _ := callGov(t, h, dir, map[string]any{"kind": "nope"})
		require.True(t, res.IsError)
		assert.Contains(t, textOf(t, res), "unknown kind")
	})
	t.Run("an enforced project without a lock reports the missing lock", func(t *testing.T) {
		enforced := governanceProject(t, "\n[lock]\nenforce = true\n")
		_, doc := callGov(t, h, enforced, nil)
		assert.Equal(t, false, doc["in_sync"])
		assert.NotEmpty(t, doc["changes"])
	})
}

// The tools resolve remote includes from the cache only. A project with an
// uncached remote include must load without starting git or touching the
// network; the control proves the instrumentation would notice a fetch.
func TestGovernanceTools_NeverFetch(t *testing.T) {
	var proxied atomic.Int64
	proxy := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { proxied.Add(1) }))
	defer proxy.Close()
	marker := filepath.Join(t.TempDir(), "git-called")
	shimDir := t.TempDir()
	realGit, err := exec.LookPath("git")
	require.NoError(t, err)
	// Only the network verbs are recorded (and fail); local git calls pass through.
	shim := "#!/bin/sh\ncase \" $* \" in\n *\" ls-remote \"*|*\" fetch \"*|*\" clone \"*|*\" pull \"*) echo \"$@\" >> " + marker + "; exit 1;;\nesac\nexec " + realGit + " \"$@\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(shimDir, "git"), []byte(shim), 0o755)) //nolint:gosec // test shim must be executable
	t.Setenv("PATH", shimDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("HTTPS_PROXY", proxy.URL)
	dir := governanceProject(t, "\n[[includes]]\nname = \"shared\"\nsource = \"https://example.invalid/shared.git\"\nref = \"main\"\n")

	// Control: a load that is allowed to fetch does reach git.
	_, _ = loadWithResolvers(context.Background(), dir) //nolint:errcheck // a failed include fetch is only a warning
	_, statErr := os.Stat(marker)
	require.NoError(t, statErr, "the control load must have called the git shim")
	require.NoError(t, os.Remove(marker))

	tests := []struct {
		name string
		h    func(context.Context, *ToolRequest) (*sdkmcp.CallToolResult, error)
		args map[string]any
	}{
		{"list_roles", ListRolesHandler, nil},
		{"resolve_role", ResolveRoleHandler, map[string]any{"role": "ops"}},
		{"lock_status", LockStatusHandler("test", nil), nil},
		{"catalog", CatalogHandler("test"), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			res, _ := callGov(t, tt.h, dir, tt.args)

			// Assert
			require.False(t, res.IsError, textOf(t, res))
			_, statErr := os.Stat(marker)
			called, _ := os.ReadFile(marker) //nolint:errcheck // absent when git never ran
			assert.True(t, os.IsNotExist(statErr), "git must not run, ran: %s", called)
			assert.Zero(t, proxied.Load(), "no HTTP request may leave")
			if tt.name != "lock_status" { // its diff already carries the skipped-output note
				require.Len(t, res.Content, 2)
				assert.True(t, strings.Contains(res.Content[1].(*sdkmcp.TextContent).Text, "local cache only"))
			}
		})
	}
}

package mcp_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lockrun"
	"github.com/Goldziher/ai-rulez/v5/internal/mcp"
)

const lockedConfig = "version = \"5.0\"\nname = \"p\"\ngitignore = false\npresets = [\"claude\"]\n"

func writeProjectFile(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
}

func callText(ctx context.Context, t *testing.T, s *sdkmcp.ClientSession, tool string) (string, bool) {
	t.Helper()
	res, err := s.CallTool(ctx, &sdkmcp.CallToolParams{Name: tool, Arguments: map[string]any{}})
	require.NoError(t, err)
	var text strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*sdkmcp.TextContent); ok {
			text.WriteString(tc.Text)
		}
	}
	return text.String(), res.IsError
}

// A user-only skill is not content the model may invoke, but it is part of the
// configuration, and the lock pins it. The audit tools therefore report over all of
// it: a fully locked project with a user-only skill is in sync, valid and free of
// lock errors, as the command line reports it, while the content and listing
// surfaces still refuse and omit the skill.
func TestAuditTools_LockedProjectWithAUserInvokedSkillIsInSync(t *testing.T) {
	// Arrange
	root := t.TempDir()
	writeProjectFile(t, root, ".ai-rulez/config.toml", lockedConfig)
	writeProjectFile(t, root, ".ai-rulez/skills/rev/SKILL.md",
		"---\ndescription: A user-only skill to deploy\ndelivery: both\ndisable-model-invocation: true\n---\n\n# rev\n")
	writeProjectFile(t, root, ".ai-rulez/skills/review/SKILL.md",
		"---\ndescription: An open review skill for the team\ndelivery: both\n---\n\n# review\n")
	env := lockrun.Env{Version: "test", Cwd: root, Load: func(ctx context.Context, p config.LockPolicy, opts ...config.LoadOption) (*config.Config, error) {
		return config.LoadConfig(ctx, root, append(opts, config.WithLockPolicy(p), config.WithoutLocal())...)
	}}
	_, err := lockrun.Write(context.Background(), lockrun.Request{}, env)
	require.NoError(t, err)

	setup := &mcp.ServeSetup{WorkDir: root, NoWatch: true, CacheDir: filepath.Join(t.TempDir(), "cache"),
		Authoring: []mcp.Option{mcp.WithRoot(root)}}
	srv, err := setup.NewServer(context.Background())
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	serverT, clientT := sdkmcp.NewInMemoryTransports()
	go func() { _ = srv.GetMCPServer().Run(ctx, serverT) }()
	session, err := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "c", Version: "1"}, nil).Connect(ctx, clientT, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })

	// Act
	lockText, lockErr := callText(ctx, t, session, "lock_status")
	catalogText, catalogErr := callText(ctx, t, session, "catalog")
	doctorText, doctorErr := callText(ctx, t, session, "doctor")
	listText, _ := callText(ctx, t, session, "list_skills")
	readRes, err := session.CallTool(ctx, &sdkmcp.CallToolParams{Name: "read_skill", Arguments: map[string]any{"name": "rev"}})
	require.NoError(t, err)

	// Assert: the audit is whole and in sync.
	require.False(t, lockErr, lockText)
	var lock struct {
		InSync  bool  `json:"in_sync"`
		Changes []any `json:"changes"`
	}
	require.NoError(t, json.Unmarshal([]byte(lockText), &lock))
	assert.True(t, lock.InSync, lockText)
	assert.Empty(t, lock.Changes)

	require.False(t, catalogErr, catalogText)
	var catalog struct {
		Lock struct {
			SourcesInSync *bool `json:"sources_in_sync"`
		} `json:"lock"`
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte(catalogText), &catalog))
	require.NotNil(t, catalog.Lock.SourcesInSync)
	assert.True(t, *catalog.Lock.SourcesInSync, catalogText)
	var ids []string
	for _, it := range catalog.Items {
		ids = append(ids, it.ID)
	}
	assert.Contains(t, ids, "rev", "the audit lists the skill by name")

	require.False(t, doctorErr, doctorText)
	assert.NotContains(t, strings.ToLower(doctorText), "source removed")
	assert.NotContains(t, strings.ToLower(doctorText), "orphan")

	// Assert: content and listing surfaces still keep the skill from the model.
	assert.NotContains(t, listText, `"rev"`)
	assert.Contains(t, listText, `"review"`)
	require.True(t, readRes.IsError)
	assert.Contains(t, readRes.Content[0].(*sdkmcp.TextContent).Text, "is user-invoked only")
}

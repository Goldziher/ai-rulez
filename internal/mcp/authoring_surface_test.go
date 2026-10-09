package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync/atomic"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthoringServerOffersPrompts(t *testing.T) {
	t.Parallel()
	session := connect(t, NewServer("test", WithAnyDirectory()))

	var names []string
	for p, err := range session.Prompts(context.Background(), nil) {
		require.NoError(t, err)
		names = append(names, p.Name)
		assert.NotEmpty(t, p.Title)
		assert.NotEmpty(t, p.Description)
	}

	assert.ElementsMatch(t, []string{"author-skill", "add-rule", "review-config", "trim-context"}, names)
}

func TestPromptsExpandTheirArguments(t *testing.T) {
	t.Parallel()
	session := connect(t, NewServer("test", WithAnyDirectory()))

	got, err := session.GetPrompt(context.Background(), &sdkmcp.GetPromptParams{
		Name: "author-skill", Arguments: map[string]string{"name": "deploy-app", "purpose": "ship to staging"},
	})

	require.NoError(t, err)
	require.Len(t, got.Messages, 1)
	text, ok := got.Messages[0].Content.(*sdkmcp.TextContent)
	require.True(t, ok)
	assert.Contains(t, text.Text, `"deploy-app"`)
	assert.Contains(t, text.Text, "ship to staging")

	_, err = session.GetPrompt(context.Background(), &sdkmcp.GetPromptParams{Name: "author-skill", Arguments: map[string]string{"name": "x"}})
	assert.Error(t, err, "a required argument is enforced")
}

// A prompt that names a tool the server does not have sends the model to a dead end.
func TestPromptsNameOnlyRealTools(t *testing.T) {
	t.Parallel()
	session := connect(t, NewServer("test", WithAnyDirectory()))
	tools := map[string]bool{}
	for tool, err := range session.Tools(context.Background(), nil) {
		require.NoError(t, err)
		tools[tool.Name] = true
	}
	toolName := regexp.MustCompile(`\b([a-z]+_[a-z_]+)\b`)

	for _, spec := range authoringPrompts() {
		args := map[string]string{"name": "n", "purpose": "p", "guideline": "g", "focus": "f", "budget": "100"}
		for _, m := range toolName.FindAllStringSubmatch(spec.text(args), -1) {
			assert.True(t, tools[m[1]], "prompt %s names %q, which is not a tool", spec.name, m[1])
		}
	}
}

func TestResourcesServeConfigCatalogAndItems(t *testing.T) {
	t.Parallel()
	root := telemetryProject(t)
	session := connect(t, NewServer("test", WithRoot(root)))
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".ai-rulez", "domains", "api", "rules"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".ai-rulez", "domains", "api", "rules", "versioning.md"), []byte("# Versioning\n"), 0o600))
	read := func(uri string) (*sdkmcp.ReadResourceResult, error) {
		return session.ReadResource(context.Background(), &sdkmcp.ReadResourceParams{URI: uri})
	}

	var listed []string
	for r, err := range session.Resources(context.Background(), nil) {
		require.NoError(t, err)
		listed = append(listed, r.URI)
	}
	assert.ElementsMatch(t, []string{"ai-rulez://config", "ai-rulez://catalog"}, listed)
	var templates []string
	for r, err := range session.ResourceTemplates(context.Background(), nil) {
		require.NoError(t, err)
		templates = append(templates, r.URITemplate)
	}
	assert.ElementsMatch(t, []string{"ai-rulez://{kind}/{name}", "ai-rulez://domains/{domain}/{kind}/{name}"}, templates)

	cfg, err := read("ai-rulez://config")
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, json.Unmarshal([]byte(cfg.Contents[0].Text), &doc))
	assert.Equal(t, "t", doc["name"])

	rule, err := read("ai-rulez://rules/atomic-commits")
	require.NoError(t, err)
	assert.Equal(t, "# Atomic\n", rule.Contents[0].Text)
	assert.Equal(t, "text/markdown", rule.Contents[0].MIMEType)

	domain, err := read("ai-rulez://domains/api/rules/versioning")
	require.NoError(t, err)
	assert.Equal(t, "# Versioning\n", domain.Contents[0].Text)

	catalog, err := read("ai-rulez://catalog")
	require.NoError(t, err)
	assert.Contains(t, catalog.Contents[0].Text, "atomic-commits")

	for name, uri := range map[string]string{
		"an unknown kind":    "ai-rulez://widgets/x",
		"a missing item":     "ai-rulez://rules/nope",
		"a foreign scheme":   "file:///etc/passwd",
		"a control char uri": "ai-rulez://rules/a\x01b",
		"a traversal":        "ai-rulez://rules/..",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := read(uri)
			assert.Error(t, err)
		})
	}
}

func TestResourcesNeedAnAllowedRoot(t *testing.T) {
	t.Parallel()
	session := connect(t, NewServer("test"))

	_, err := session.ReadResource(context.Background(), &sdkmcp.ReadResourceParams{URI: "ai-rulez://config"})

	assert.Error(t, err)
}

func TestCacheHintsFollowWhatCanChange(t *testing.T) {
	t.Parallel()
	hint := func(req sdkmcp.Request) sdkmcp.Cacheable {
		c := sdkmcp.Cacheable{}
		authoringCacheable(context.Background(), req, &c)
		return c
	}

	tools := hint(&sdkmcp.ListToolsRequest{})
	assert.Equal(t, staticTTLMs, tools.TTLMs)
	assert.Equal(t, "public", tools.CacheScope)
	read := hint(&sdkmcp.ReadResourceRequest{})
	assert.Zero(t, read.TTLMs, "project content can change between reads")
	assert.Equal(t, "private", read.CacheScope)

	static := NewSkillServerWith("test", loadCatalog(t), ServeOptions{})
	live := NewSkillServerWith("test", loadCatalog(t), ServeOptions{Rebuild: func() (*Catalog, error) { return nil, nil }, Fingerprint: func() (string, error) { return "", nil }})
	servedHint := func(s *Server) sdkmcp.Cacheable {
		c := sdkmcp.Cacheable{}
		s.servingCacheable(context.Background(), &sdkmcp.ReadResourceRequest{}, &c)
		return c
	}
	assert.Equal(t, servedTTLMs, servedHint(static).TTLMs)
	assert.Zero(t, servedHint(live).TTLMs, "a live reload can change any served file")
}

// A long call reports progress when the client asks, and stops when it cancels.
func TestRecursiveGenerateReportsProgressAndStopsOnCancel(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	const projects = 60
	for i := range projects {
		dir := filepath.Join(root, fmt.Sprintf("svc%02d", i), ".ai-rulez", "rules")
		require.NoError(t, os.MkdirAll(dir, 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "..", "config.toml"), []byte("version = \"5.0\"\nname = \"t\"\npresets = [\"claude\"]\nagents_md = false\n"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "r.md"), []byte("# R\n"), 0o600))
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	callCtx, cancelCall := context.WithCancel(ctx)
	t.Cleanup(cancelCall)
	var seen atomic.Int32
	serverT, clientT := sdkmcp.NewInMemoryTransports()
	srv := NewServer("test", WithAnyDirectory())
	go func() { _ = srv.GetMCPServer().Run(ctx, serverT) }()
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "c", Version: "1"}, &sdkmcp.ClientOptions{
		ProgressNotificationHandler: func(context.Context, *sdkmcp.ProgressNotificationClientRequest) {
			if seen.Add(1) == 2 {
				cancelCall() // the user gave up
			}
		},
	})
	session, err := client.Connect(ctx, clientT, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	params := &sdkmcp.CallToolParams{Name: "generate_outputs", Arguments: map[string]any{"working_directory": root, "recursive": true}}
	params.SetProgressToken("p1")

	_, err = session.CallTool(callCtx, params)

	require.Error(t, err, "the cancelled call does not complete")
	assert.GreaterOrEqual(t, int(seen.Load()), 2, "progress notifications arrived")
	last := filepath.Join(root, fmt.Sprintf("svc%02d", projects-1), "CLAUDE.md")
	assert.Never(t, func() bool {
		_, statErr := os.Stat(last)
		return statErr == nil
	}, 3*time.Second, 100*time.Millisecond, "the server kept generating after the client cancelled")
}

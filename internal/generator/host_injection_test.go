package generator

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
)

// recordingLogger collects log messages.
type recordingLogger struct{ msgs []string }

func (r *recordingLogger) Debug(msg string, _ ...any) { r.msgs = append(r.msgs, msg) }
func (r *recordingLogger) Info(msg string, _ ...any)  { r.msgs = append(r.msgs, msg) }
func (r *recordingLogger) Warn(msg string, _ ...any)  { r.msgs = append(r.msgs, msg) }
func (r *recordingLogger) Error(msg string, _ ...any) { r.msgs = append(r.msgs, msg) }

func hostProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, ".ai-rulez")
	require.NoError(t, os.MkdirAll(filepath.Join(cfgDir, "rules"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte(`version = "4.0"
name = "host"
presets = ["claude"]
gitignore = false

[header]
timestamp = true

[[mcp_servers]]
name = "tool"
command = "tool"
[mcp_servers.env]
TOKEN = "${HOST_TEST_TOKEN}"
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "rules", "r.md"), []byte("# R\n"), 0o644))
	return dir
}

func TestGenerateUsesTheInjectedHost(t *testing.T) {
	// Arrange: nothing of the real process is available to the run
	t.Setenv("HOST_TEST_TOKEN", "from-the-real-process")
	t.Setenv("SOURCE_DATE_EPOCH", "1")
	dir := hostProject(t)
	log := &recordingLogger{}
	host := ambient.Host{
		Env:    ambient.MapEnv{Vars: map[string]string{"HOST_TEST_TOKEN": "injected-value", "SOURCE_DATE_EPOCH": "1700000000"}},
		Clock:  ambient.Fixed(time.Unix(5, 0)),
		Runner: runner.Deny{},
		Log:    log,
	}
	cfg, err := config.LoadConfig(context.Background(), dir, config.WithHost(host))
	require.NoError(t, err)

	// Act
	require.NoError(t, NewGenerator(cfg).Generate("default"))

	// Assert
	claude, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	require.NoError(t, err)
	assert.Contains(t, string(claude), time.Unix(1700000000, 0).Format("2006-01-02"), "SOURCE_DATE_EPOCH comes from the injected env")
	assert.True(t, containsMsg(log.msgs, "Generation complete"), "log lines go to the injected logger: %v", log.msgs)
}

func TestGenerateDoesNotSeeTheRealEnvironmentWhenOneIsInjected(t *testing.T) {
	// Arrange: the variable exists in the process but not in the injected env
	t.Setenv("HOST_TEST_TOKEN", "from-the-real-process")
	dir := hostProject(t)
	cfg, err := config.LoadConfig(context.Background(), dir, config.WithHost(ambient.Host{Env: ambient.MapEnv{}, Log: &recordingLogger{}}))
	require.NoError(t, err)

	// Act
	err = NewGenerator(cfg).Generate("default")

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unresolved MCP env placeholders")
}

func TestGeneratorSetHostOverridesTheConfigHost(t *testing.T) {
	// Arrange
	dir := hostProject(t)
	cfg, err := config.LoadConfig(context.Background(), dir, config.WithoutRemote())
	require.NoError(t, err)
	g := NewGenerator(cfg)
	fake := &runner.Fake{}

	// Act
	g.SetHost(ambient.Host{Runner: fake, Env: ambient.MapEnv{Vars: map[string]string{"HOST_TEST_TOKEN": "x"}}})
	_, _ = g.git().IsRepoContext(g.ctx, dir), g.git().TopLevelContext(g.ctx, dir)

	// Assert
	assert.Len(t, fake.Calls(), 2, "git questions go through the injected runner")
	assert.Equal(t, "x", g.host().GetEnv("HOST_TEST_TOKEN"))
}

func containsMsg(msgs []string, want string) bool {
	for _, m := range msgs {
		if strings.Contains(m, want) {
			return true
		}
	}
	return false
}

package crud_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/crud"
)

// recordingLogger keeps every log line, arguments included.
type recordingLogger struct {
	mu    sync.Mutex
	lines []string
}

func (r *recordingLogger) add(level, msg string, args []any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lines = append(r.lines, level+" "+msg+" "+fmt.Sprint(args...))
}
func (r *recordingLogger) Debug(msg string, args ...any) { r.add("debug", msg, args) }
func (r *recordingLogger) Info(msg string, args ...any)  { r.add("info", msg, args) }
func (r *recordingLogger) Warn(msg string, args ...any)  { r.add("warn", msg, args) }
func (r *recordingLogger) Error(msg string, args ...any) { r.add("error", msg, args) }
func (r *recordingLogger) text() string                  { return strings.Join(r.lines, "\n") }

const credentialedURL = "https://user:SECRETTOKEN@github.com/example/rules.git?access_token=SECRETQUERY"

func TestSourceOperationsNeverLogCredentials(t *testing.T) {
	ctx := context.Background()
	log := &recordingLogger{}
	op, err := crud.NewOperator(setupTestProject(t))
	require.NoError(t, err)
	op = op.WithLogger(log)

	// A credentialed source is refused, so the credential reaches neither the
	// config file nor a log line.
	require.Error(t, op.AddInclude(ctx, &crud.AddIncludeRequest{Name: "inc", Source: credentialedURL}))
	require.Error(t, op.InstallSkill(ctx, &crud.InstallSkillRequest{Name: "sk", Source: credentialedURL}))

	assert.NotContains(t, log.text(), "SECRETTOKEN")
	assert.NotContains(t, log.text(), "SECRETQUERY")
}

func TestAddSourceRefusesToStoreACredential(t *testing.T) {
	base := setupTestProject(t)
	op, err := crud.NewOperator(base)
	require.NoError(t, err)

	err = op.AddInclude(context.Background(), &crud.AddIncludeRequest{Name: "inc", Source: credentialedURL})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "embeds a credential")
	data, readErr := os.ReadFile(filepath.Join(base, ".ai-rulez", "config.toml"))
	require.NoError(t, readErr)
	assert.NotContains(t, string(data), "SECRETTOKEN")
	assert.NotContains(t, string(data), "SECRETQUERY")
}

func TestInvalidCredentialedSourceErrorsAreRedacted(t *testing.T) {
	op, err := crud.NewOperator(setupTestProject(t))
	require.NoError(t, err)

	err = op.AddInclude(context.Background(), &crud.AddIncludeRequest{Name: "inc", Source: "http://user:SECRETTOKEN@example.com/o/r.git"})

	require.Error(t, err)
	assert.NotContains(t, fmt.Sprintf("%+v", err), "SECRETTOKEN")
}

func TestAddIncludeRefusesALocalPathOutsideTheProject(t *testing.T) {
	base := setupTestProject(t)
	outside := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(outside, ".ai-rulez"), 0o755))
	op, err := crud.NewOperator(base)
	require.NoError(t, err)

	err = op.AddInclude(context.Background(), &crud.AddIncludeRequest{Name: "out", Source: outside})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "outside the project")
	data, readErr := os.ReadFile(filepath.Join(base, ".ai-rulez", "config.toml"))
	require.NoError(t, readErr)
	assert.NotContains(t, string(data), "out")
}

func TestRemoveIncludeWorksOnAnUnloadableConfig(t *testing.T) {
	ctx := context.Background()
	base := setupTestProject(t)
	cfgPath := filepath.Join(base, ".ai-rulez", "config.toml")
	outside := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(outside, ".ai-rulez"), 0o755))
	bad := fmt.Sprintf("version = \"5.0\"\nname = \"test-project\"\n\n[[includes]]\nname = \"bad\"\nsource = %q\n\n[[installed_skills]]\nname = \"sk\"\nsource = \"https://github.com/o/r.git\"\n", outside)
	require.NoError(t, os.WriteFile(cfgPath, []byte(bad), 0o644))
	op, err := crud.NewOperator(base)
	require.NoError(t, err)

	require.NoError(t, op.RemoveInclude(ctx, "bad"))
	require.NoError(t, op.UninstallSkill(ctx, "sk"))

	data, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "bad")
	assert.NotContains(t, string(data), "installed_skills")
	assert.Contains(t, string(data), "test-project")
	require.Error(t, op.RemoveInclude(ctx, "bad"), "removing it twice reports that it is gone")
}

package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resetExportFlags(t *testing.T) {
	t.Helper()
	clear := func() {
		usageExportTo, usageExportFile, usageExportDryRun, usageLog = "", "", false, ""
		usageExportAll, usageExportMaxBatches = false, 10
	}
	clear()
	t.Cleanup(clear)
}

func TestUsageExport(t *testing.T) {
	t.Run("should write a deterministic allowlisted file from the log", func(t *testing.T) {
		resetExportFlags(t)
		env := setupTelemetry(t, "", "")
		recordSkillLoads(t, env, "deploy", "release-notes")
		logLine := `{"v":3,"ts":"2026-10-03T08:00:00Z","event":"skill_invoked","skill":"extra","id":"extra","invocation":"tool","harness":"claude","prompt":"TOPSECRET","cwd":"/home/TOPSECRET"}` + "\n"
		f, err := os.OpenFile(env.log, os.O_APPEND|os.O_WRONLY, 0o600)
		require.NoError(t, err)
		_, err = f.WriteString(logLine)
		require.NoError(t, err)
		require.NoError(t, f.Close())
		dest := filepath.Join(t.TempDir(), "out", "usage.ndjson")
		usageExportTo = "file"
		var out bytes.Buffer

		require.NoError(t, runUsageExport(&out, []string{dest}))
		first, err := os.ReadFile(dest)
		require.NoError(t, err)
		require.NoError(t, runUsageExport(&out, []string{dest}))
		second, err := os.ReadFile(dest)
		require.NoError(t, err)

		assert.Equal(t, first, second, "the same log gives the same file")
		assert.Contains(t, out.String(), "wrote 3 events in 1 batches to "+dest)
		lines := strings.Split(strings.TrimSuffix(string(first), "\n"), "\n")
		require.Len(t, lines, 1)
		for _, id := range []string{"deploy", "release-notes", "extra"} {
			assert.Contains(t, lines[0], `"stringValue":"`+id+`"`)
		}
		assert.NotContains(t, string(first), "TOPSECRET", "keys outside the allowlist never reach the file")
		assert.NotContains(t, string(first), `"ai_rulez.session"`, "the session stays out unless include_session is on")
		assert.NotContains(t, string(first), "resourceMetrics")
		if runtime.GOOS != "windows" {
			info, statErr := os.Stat(dest)
			require.NoError(t, statErr)
			assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
		}
	})

	t.Run("should include the session when the user scope opens it", func(t *testing.T) {
		resetExportFlags(t)
		env := setupTelemetry(t, "", "[telemetry]\ninclude_session = true\n")
		recordSkillLoads(t, env, "deploy")
		dest := filepath.Join(t.TempDir(), "usage.ndjson")
		usageExportTo, usageExportFile = "file", dest

		require.NoError(t, runUsageExport(&bytes.Buffer{}, nil))

		data, err := os.ReadFile(dest)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"ai_rulez.session"`)
	})

	t.Run("should write nothing on a dry run", func(t *testing.T) {
		resetExportFlags(t)
		env := setupTelemetry(t, "", "")
		recordSkillLoads(t, env, "deploy")
		dest := filepath.Join(t.TempDir(), "usage.ndjson")
		usageExportTo, usageExportDryRun = "file", true
		var out bytes.Buffer

		require.NoError(t, runUsageExport(&out, []string{dest}))

		assert.Contains(t, out.String(), "would write 1 events")
		assert.NoFileExists(t, dest)
	})

	t.Run("should replace a symlinked destination instead of writing through it", func(t *testing.T) {
		resetExportFlags(t)
		env := setupTelemetry(t, "", "")
		recordSkillLoads(t, env, "deploy")
		dir := t.TempDir()
		victim := filepath.Join(dir, "victim.txt")
		require.NoError(t, os.WriteFile(victim, []byte("keep"), 0o600))
		dest := filepath.Join(dir, "usage.ndjson")
		testutil.SymlinkOrSkip(t, victim, dest)
		usageExportTo = "file"

		require.NoError(t, runUsageExport(&bytes.Buffer{}, []string{dest}))

		kept, err := os.ReadFile(victim)
		require.NoError(t, err)
		assert.Equal(t, "keep", string(kept))
		info, err := os.Lstat(dest)
		require.NoError(t, err)
		assert.True(t, info.Mode().IsRegular())
	})

	t.Run("should refuse bad invocations", func(t *testing.T) {
		tests := []struct {
			name string
			set  func(env telemetryEnv)
			args []string
			want string
		}{
			{"no destination kind", func(telemetryEnv) {}, []string{"x"}, "--to must be file"},
			{"otlp takes no destination path", func(telemetryEnv) { usageExportTo = "otlp" }, []string{"x"}, "destination path belongs to --to file"},
			{"otlp needs consent", func(telemetryEnv) { usageExportTo = "otlp" }, nil, "OTLP export is not active"},
			{"no path", func(telemetryEnv) { usageExportTo = "file" }, nil, "destination path is required"},
			{"two different paths", func(telemetryEnv) { usageExportTo, usageExportFile = "file", "a" }, []string{"b"}, "not both"},
			{"destination is the log", func(env telemetryEnv) { usageExportTo = "file"; usageLog = env.log }, []string{"LOG"}, "is the usage log"},
			{"missing log", func(telemetryEnv) { usageExportTo = "file" }, []string{"out.ndjson"}, "usage log"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				resetExportFlags(t)
				env := setupTelemetry(t, "", "")
				if tt.name == "destination is the log" {
					require.NoError(t, os.MkdirAll(filepath.Dir(env.log), 0o750))
					require.NoError(t, os.WriteFile(env.log, []byte("{}\n"), 0o600))
					tt.args = []string{env.log}
				}
				tt.set(env)
				if len(tt.args) == 1 && tt.args[0] == "out.ndjson" {
					tt.args[0] = filepath.Join(t.TempDir(), "out.ndjson")
				}

				err := runUsageExport(&bytes.Buffer{}, tt.args)

				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.want)
			})
		}
	})
}

func TestUsageExport_DefaultLogIsTheOnePreviewReads(t *testing.T) {
	resetExportFlags(t)
	env := setupTelemetry(t, "", "")
	recordSkillLoads(t, env, "deploy")
	sub := filepath.Join(env.root, "pkg", "deep")
	require.NoError(t, os.MkdirAll(sub, 0o750))
	t.Setenv("CLAUDE_PROJECT_DIR", "") // as outside a Claude session: the project is found from the working directory
	chdir(t, sub)
	dest := filepath.Join(t.TempDir(), "usage.ndjson")
	usageExportTo = "file"
	var out bytes.Buffer

	require.NoError(t, runUsageExport(&out, []string{dest}))

	assert.Contains(t, out.String(), "wrote 1 events", "the project's log is found from a subdirectory, as `telemetry preview` finds it")
}

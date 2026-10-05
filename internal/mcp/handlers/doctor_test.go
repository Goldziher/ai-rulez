package handlers

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/doctor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDoctorHandler(t *testing.T) {
	tests := []struct {
		name         string
		config       string
		strict       bool
		wantOK       bool
		wantCheck    string
		wantSeverity string
	}{
		{name: "removed preset is an error", config: "version = \"4.0\"\nname = \"x\"\npresets = [\"windsurf\"]\n", wantOK: false, wantCheck: "presets", wantSeverity: "error"},
		{name: "ungenerated output is a warning that passes", config: generateSharedConfig, wantOK: true, wantCheck: "drift", wantSeverity: "warning"},
		{name: "strict turns the warning into a failure", config: generateSharedConfig, strict: true, wantOK: false, wantCheck: "drift", wantSeverity: "warning"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := t.TempDir()
			require.NoError(t, os.MkdirAll(filepath.Join(dir, ".ai-rulez"), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(dir, ".ai-rulez", "config.toml"), []byte(tt.config), 0o600))

			// Act
			res, err := DoctorHandler(context.Background(), newRequestWithArgs(map[string]any{"working_directory": dir, "strict": tt.strict}))

			// Assert
			require.NoError(t, err)
			require.False(t, res.IsError, textOf(t, res))
			var out struct {
				OK       bool `json:"ok"`
				Findings []struct {
					Check    string `json:"check"`
					Severity string `json:"severity"`
				} `json:"findings"`
			}
			require.NoError(t, json.Unmarshal([]byte(textOf(t, res)), &out))
			assert.Equal(t, tt.wantOK, out.OK)
			found := false
			for _, f := range out.Findings {
				found = found || (f.Check == tt.wantCheck && f.Severity == tt.wantSeverity)
			}
			assert.True(t, found, "want a %s %s finding in %+v", tt.wantSeverity, tt.wantCheck, out.Findings)
		})
	}
}

func TestRedactReport_CoversPathAndRoot(t *testing.T) {
	// Arrange
	report := &doctor.Report{
		Root: "https://user:secret@example.com/root",
		Findings: []doctor.Finding{{
			Message: "fetch https://user:secret@example.com/a failed",
			Hint:    "see https://user:secret@example.com/b",
			Path:    "https://user:secret@example.com/c",
		}},
	}

	// Act
	redactReport(report)

	// Assert
	out, err := json.Marshal(report)
	require.NoError(t, err)
	assert.NotContains(t, string(out), "secret")
}

func TestDoctorHandler_WritesNothingAndSkipsIncludes(t *testing.T) {
	// Arrange: an include that cannot be resolved would fail a full load, and a
	// remote one would be fetched and cached.
	dir := t.TempDir()
	cfg := generateSharedConfig + "\n[[includes]]\nname = \"gone\"\nsource = \"./does-not-exist\"\n"
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".ai-rulez"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".ai-rulez", "config.toml"), []byte(cfg), 0o600))
	snapshot := func() map[string]int64 {
		files := map[string]int64{}
		require.NoError(t, filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				info, infoErr := d.Info()
				require.NoError(t, infoErr)
				files[path] = info.Size()
			}
			return err
		}))
		return files
	}
	before := snapshot()

	// Act
	res, err := DoctorHandler(context.Background(), newRequestWithArgs(map[string]any{"working_directory": dir}))

	// Assert
	require.NoError(t, err)
	require.False(t, res.IsError, textOf(t, res))
	assert.Contains(t, textOf(t, res), "not checked")
	assert.Equal(t, before, snapshot())
}

package contentlock

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

func TestComputePinsRubricDirectories(t *testing.T) {
	// Arrange
	f := newFixture(t)
	f.write("rubrics/mine/rubric.toml", []byte("id = \"mine\"\n"), 0o644)
	f.write("rubrics/mine/golden/a.golden.yaml", []byte("id: a\n"), 0o644)
	f.write("rubrics/other/rubric.toml", []byte("id = \"other\"\n"), 0o644)

	// Act
	snap, err := Compute(f.cfg, Options{})

	// Assert
	require.NoError(t, err)
	got := map[string]string{}
	for _, it := range snap.Items {
		if it.Kind == KindRubric {
			got[it.ID] = it.Path
		}
	}
	assert.Equal(t, map[string]string{"mine": "rubrics/mine", "other": "rubrics/other"}, got)
	assert.Empty(t, snap.Problems)
}

func TestRubricPinChangesWithAnyFileOfTheRubric(t *testing.T) {
	tests := []struct {
		name string
		edit func(f *fixture)
	}{
		{"rubric.toml", func(f *fixture) { f.write("rubrics/mine/rubric.toml", []byte("id = \"mine\"\nversion = 2\n"), 0o644) }},
		{"system prompt", func(f *fixture) { f.write("rubrics/mine/system.md", []byte("be lenient"), 0o644) }},
		{"golden label", func(f *fixture) {
			f.write("rubrics/mine/golden/a.golden.yaml", []byte("id: a\nadjudicated: {x: pass}\n"), 0o644)
		}},
		{"calibration record", func(f *fixture) { f.write("rubrics/mine/calibration.json", []byte(`{"status":"pass"}`), 0o644) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			f := newFixture(t)
			f.write("rubrics/mine/rubric.toml", []byte("id = \"mine\"\n"), 0o644)
			f.write("rubrics/mine/golden/a.golden.yaml", []byte("id: a\n"), 0o644)
			f.write("rubrics/other/rubric.toml", []byte("id = \"other\"\n"), 0o644)
			before := f.items()

			// Act
			tt.edit(f)
			after := f.items()

			// Assert
			assert.NotEqual(t, before["rubric:/mine"], after["rubric:/mine"])
			assert.Equal(t, before["rubric:/other"], after["rubric:/other"], "an untouched rubric keeps its pin")
		})
	}
}

func TestRubricEditIsLockDrift(t *testing.T) {
	// Arrange
	f := newFixture(t)
	f.write("rubrics/mine/rubric.toml", []byte("id = \"mine\"\n"), 0o644)
	snap, err := Compute(f.cfg, Options{})
	require.NoError(t, err)
	var lock lockfile.File
	lock.Version = lockfile.Version
	Build(&lock, snap)

	// Act
	f.write("rubrics/mine/rubric.toml", []byte("id = \"mine\"\nversion = 9\n"), 0o644)
	now, err := Compute(f.cfg, Options{})
	require.NoError(t, err)
	diff := Compare(&lock, now)

	// Assert
	require.False(t, diff.InSync)
	require.Len(t, diff.Changes, 1)
	assert.Equal(t, KindRubric, diff.Changes[0].Kind)
	assert.Equal(t, "mine", diff.Changes[0].ID)
}

func TestRubricSymlinkIsAProblemNotASkippedFile(t *testing.T) {
	// Arrange
	f := newFixture(t)
	f.write("rubrics/mine/rubric.toml", []byte("id = \"mine\"\n"), 0o644)
	secret := filepath.Join(f.root, "outside.txt")
	require.NoError(t, os.WriteFile(secret, []byte("outside"), 0o600))
	testutil.SymlinkOrSkip(t, secret, filepath.Join(f.cfg.ConfigDir, "rubrics", "mine", "system.md"))

	// Act
	snap, err := Compute(f.cfg, Options{})

	// Assert
	require.NoError(t, err)
	require.Len(t, snap.Problems, 1)
	assert.Contains(t, snap.Problems[0], "not a regular file")
}

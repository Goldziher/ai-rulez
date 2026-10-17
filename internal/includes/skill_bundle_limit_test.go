package includes

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func TestScanEntrySkills_SkillOverTheBundleCapFailsTheEntry(t *testing.T) {
	tests := []struct {
		name  string
		multi bool
	}{
		{"a single skill entry", false},
		{"one skill among several", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			base := t.TempDir()
			skill := base
			if tt.multi {
				skill = filepath.Join(base, "big")
				other := filepath.Join(base, "ok")
				require.NoError(t, os.MkdirAll(other, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(other, "SKILL.md"), []byte("---\nname: ok\ndescription: d\n---\nb\n"), 0o644))
			}
			require.NoError(t, os.MkdirAll(filepath.Join(skill, "docs"), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("---\nname: big\ndescription: d\n---\nb\n"), 0o644))
			for i := range 2001 {
				require.NoError(t, os.WriteFile(filepath.Join(skill, "docs", fmt.Sprintf("f%04d.md", i)), []byte("x"), 0o644))
			}

			// Act
			_, err := scanEntrySkills(context.Background(), base, false, "", "entry", nil, nil)

			// Assert
			var limit *config.SkillBundleLimitError
			require.True(t, errors.As(err, &limit), err)
		})
	}
}

func writeSizedSkill(t *testing.T, dir, name string, files, size int) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "docs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: "+name+"\ndescription: d\n---\nb\n"), 0o644))
	for i := range files {
		p := filepath.Join(dir, "docs", fmt.Sprintf("f%04d.bin", i))
		f, err := os.Create(p)
		require.NoError(t, err)
		require.NoError(t, f.Truncate(int64(size))) // sparse: costs no disk
		require.NoError(t, f.Close())
	}
}

func TestScanEntrySkills_EntryTotalIsCapped(t *testing.T) {
	tests := []struct {
		name          string
		skills, files int
		size          int
		wantErr       string
	}{
		{"bytes across many mid-size skills", 5, 2, 7 << 20, "64 MiB"},
		{"files across many skills", 11, 1900, 1, "20000 files"},
		{"under both caps", 4, 2, 7 << 20, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			base := t.TempDir()
			for i := range tt.skills {
				name := fmt.Sprintf("s%02d", i)
				writeSizedSkill(t, filepath.Join(base, name), name, tt.files, tt.size)
			}
			ctx := config.WithSkillBundleBudget(context.Background())

			// Act
			out, err := scanEntrySkills(ctx, base, false, "", "entry", nil, nil)

			// Assert
			if tt.wantErr == "" {
				require.NoError(t, err)
				require.Len(t, out, tt.skills)
				return
			}
			var limit *config.SkillBundleLimitError
			require.True(t, errors.As(err, &limit), err)
			require.Contains(t, err.Error(), tt.wantErr)
			require.Contains(t, err.Error(), "include")
		})
	}
}

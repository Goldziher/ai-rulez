package commands

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/approval"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

// TestApproveGit_ServedLocalSkillIsRecomputedAtTheReviewedCommit is RV-GOV-7: a
// served skill authored in the project is digested from its files at the
// reviewed commit, never read from the lock committed there.
func TestApproveGit_ServedLocalSkillIsRecomputedAtTheReviewedCommit(t *testing.T) {
	forged := "sha256:" + strings.Repeat("cd", 32)
	tests := []struct {
		name       string
		skill      bool
		wantPinned bool
	}{
		{name: "a forged pin for a skill with no files is not pinned", skill: false, wantPinned: false},
		{name: "a forged pin for a real skill yields the files' digest", skill: true, wantPinned: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root, _, _ := reviewedProject(t, "")
			dir := filepath.Join(root, ".ai-rulez")
			want := ""
			if tt.skill {
				skill := filepath.Join(dir, "skills", "ghost", "SKILL.md")
				require.NoError(t, os.MkdirAll(filepath.Dir(skill), 0o755))
				require.NoError(t, os.WriteFile(skill, []byte("---\ndescription: Served ghost\ndelivery: served\n---\nGHOST\n"), 0o644))
				require.Equal(t, 0, writeLockAt("", "", nil))
				for _, e := range loadLockAt(t, root).Served {
					if e.Name == "ghost" {
						want = e.Digest
					}
				}
				require.NotEmpty(t, want, "lock pins the served skill")
			}
			lock := loadLockAt(t, root)
			var kept []lockfile.Entry
			for _, e := range lock.Served {
				if e.Name != "ghost" {
					kept = append(kept, e)
				}
			}
			lock.Served = append(kept, lockfile.Entry{Name: "ghost", Source: "skills/ghost", Digest: forged})
			require.NoError(t, lockfile.Save(dir, lock))
			crossGit(t, root, "add", "-A")
			crossGit(t, root, "commit", "-qm", "hand-edited served pin")
			head := crossGit(t, root, "rev-parse", "HEAD")
			g, err := newApproveGit(context.Background(), &config.Config{BaseDir: root, ConfigDir: dir})
			require.NoError(t, err)
			s := approval.Subject{Kind: approval.KindServed, ID: "ghost", Digest: forged, Class: approval.ServedClass("skills/ghost", "", "")}
			require.Equal(t, approval.ClassServedLocal, s.Class)

			// Act
			got, pinned, err := g.pinnedAt(s)(context.Background(), head)

			// Assert
			require.NoError(t, err)
			assert.Equal(t, tt.wantPinned, pinned)
			assert.NotEqual(t, forged, got, "the digest never comes from the hand-edited lock")
			assert.Equal(t, want, got)
		})
	}
}

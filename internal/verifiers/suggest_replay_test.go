package verifiers

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
)

// historyProject makes a repository with a base commit and one commit per step.
func historyProject(t *testing.T, steps ...map[string]string) *config.Config {
	t.Helper()
	cfg := specProject(t, map[string]string{"src/a.go": "package a\n"}, "")
	run := func(args ...string) {
		t.Helper()
		full := append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false"}, args...)
		out, err := gitutil.CommandNoContext(cfg.BaseDir, full...).CombinedOutput()
		require.NoError(t, err, string(out))
	}
	run("init", "-q", "-b", "main")
	run("add", "-A")
	run("commit", "-q", "-m", "base")
	for i, files := range steps {
		for name, content := range files {
			full := filepath.Join(cfg.BaseDir, filepath.FromSlash(name))
			require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
			require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
		}
		run("add", "-A")
		run("commit", "-q", "-m", "change "+string(rune('1'+i)))
	}
	return cfg
}

func TestSuggest_ReplaysTheLastMergedDiffs(t *testing.T) {
	tests := []struct {
		name        string
		replay      int
		wantDiffs   int
		wantFlagged int
		wantReplay  bool
	}{
		{"every change since the base", 5, 3, 1, true},
		{"only the newest", 1, 1, 0, true},
		{"replay is off by default", 0, 0, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange: change 1 adds a TODO, changes 2 and 3 are clean.
			cfg := historyProject(t,
				map[string]string{"src/b.go": "package a\n// TODO soon\n"},
				map[string]string{"src/c.go": "package a\n"},
				map[string]string{"src/d.go": "package a\n"},
			)
			fake := suggestFake(suggestion("", proposal(nil)))

			// Act
			res, err := Suggest(context.Background(), cfg, SuggestOptions{ID: "database", Replay: tt.replay, LLM: LLMOptions{Client: fake}})

			// Assert
			require.NoError(t, err)
			require.Len(t, res.Proposals, 1)
			p := res.Proposals[0]
			require.Empty(t, p.Rejected)
			if !tt.wantReplay {
				assert.Nil(t, p.Replay)
				return
			}
			require.NotNil(t, p.Replay)
			assert.Equal(t, tt.wantDiffs, p.Replay.Diffs)
			assert.Equal(t, tt.wantFlagged, p.Replay.Flagged)
			if tt.wantFlagged > 0 {
				require.Len(t, p.Replay.FlaggedCommits, 1)
				assert.Contains(t, p.Replay.FlaggedCommits[0], "change 1")
			}
		})
	}
}

func TestSuggest_ReplayOutsideARepositoryIsNotedNotFatal(t *testing.T) {
	cfg := suggestProject(t) // a plain directory, no history

	res, err := Suggest(context.Background(), cfg, SuggestOptions{ID: "database", Replay: 5, LLM: LLMOptions{Client: suggestFake(suggestion("", proposal(nil)))}})

	require.NoError(t, err)
	require.Len(t, res.Proposals, 1)
	assert.Nil(t, res.Proposals[0].Replay)
	assert.Contains(t, strings.Join(res.Notes, "\n"), "not in a git repository")
}

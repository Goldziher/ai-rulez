package config

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingLogger struct {
	mu    sync.Mutex
	warns []string
}

func (l *recordingLogger) Debug(string, ...any) {}
func (l *recordingLogger) Info(string, ...any)  {}
func (l *recordingLogger) Error(string, ...any) {}
func (l *recordingLogger) Warn(msg string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.warns = append(l.warns, fmt.Sprint(append([]any{msg}, args...)...))
}

// A content file over the read cap is left out of the tree. Each kind says so, instead of the
// skill, rule or agent vanishing from the output with no diagnostic.
func TestScanContentTree_OversizeFileIsReported(t *testing.T) {
	huge := "---\nname: big\ndescription: huge\n---\n" + strings.Repeat("x", maxContentFileBytes+1)

	dir := t.TempDir()
	write(t, filepath.Join(dir, "skills", "ok", "SKILL.md"), "---\nname: ok\ndescription: fine\n---\nbody\n")
	write(t, filepath.Join(dir, "skills", "big", "SKILL.md"), huge)
	write(t, filepath.Join(dir, "rules", "ok.md"), "# ok\n")
	write(t, filepath.Join(dir, "rules", "big.md"), huge)
	write(t, filepath.Join(dir, "agents", "ok.md"), "---\nname: ok\ndescription: fine\n---\nbody\n")
	write(t, filepath.Join(dir, "agents", "big.md"), huge)

	log := &recordingLogger{}
	s := newIncludeScanner(context.Background(), osView(dir))
	s.log = log
	tree, err := scanContentTree(s, dir, nil)
	require.NoError(t, err)

	require.Len(t, tree.Skills, 1)
	assert.Equal(t, "ok", tree.Skills[0].Name)
	require.Len(t, tree.Rules, 1)
	require.Len(t, tree.Agents, 1)

	joined := strings.Join(log.warns, "\n")
	for _, want := range []string{
		filepath.Join("skills", "big", "SKILL.md"),
		filepath.Join("rules", "big.md"),
		filepath.Join("agents", "big.md"),
	} {
		assert.Contains(t, joined, want)
	}
	assert.Contains(t, joined, "8 MiB")
}

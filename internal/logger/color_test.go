package logger

import (
	"bytes"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestColorEnabled(t *testing.T) {
	file, err := os.Create(filepath.Join(t.TempDir(), "log"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = file.Close() })

	tests := []struct {
		name    string
		noColor string
		term    string
		w       io.Writer
		want    bool
	}{
		{"buffer keeps color", "", "xterm", &bytes.Buffer{}, true},
		{"NO_COLOR disables", "1", "xterm", &bytes.Buffer{}, false},
		{"TERM=dumb disables", "", "dumb", &bytes.Buffer{}, false},
		{"non-terminal file disables", "", "xterm", file, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			t.Setenv("NO_COLOR", tt.noColor)
			t.Setenv("TERM", tt.term)

			// Act
			got := colorEnabled(tt.w)

			// Assert
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestHandlerStripsANSIWithoutColor(t *testing.T) {
	// Arrange
	t.Setenv("NO_COLOR", "1")
	var buf bytes.Buffer

	// Act
	New(&buf, -4).Warn("hello", "path", "a/b", "error", assert.AnError)

	// Assert
	assert.NotContains(t, buf.String(), "\x1b[")
	assert.Contains(t, buf.String(), "WARN  hello")
}

// WithAttrs and WithGroup return handlers that write to the same writer, so they
// share the parent's lock: records from sibling loggers never interleave.
func TestPrettyHandlerChildrenShareTheParentLock(t *testing.T) {
	// Arrange
	h, ok := New(&bytes.Buffer{}, slog.LevelInfo).Handler().(*prettyHandler)
	require.True(t, ok)

	// Act
	withAttrs, ok1 := h.WithAttrs([]slog.Attr{slog.String("a", "1")}).(*prettyHandler)
	withGroup, ok2 := h.WithGroup("g").(*prettyHandler)

	// Assert
	require.True(t, ok1 && ok2)
	assert.Same(t, h.mu, withAttrs.mu)
	assert.Same(t, h.mu, withGroup.mu)
}

// Appending to a parent's slices must not write into the backing array a
// sibling child shares.
func TestPrettyHandlerChildrenDoNotShareSlices(t *testing.T) {
	// Arrange: a parent whose slices have spare capacity.
	parent := &prettyHandler{w: &bytes.Buffer{}, mu: &sync.Mutex{},
		attrs: make([]slog.Attr, 1, 8), groups: make([]string, 1, 8)}
	parent.attrs[0], parent.groups[0] = slog.String("p", "0"), "p"

	// Act
	a, _ := parent.WithAttrs([]slog.Attr{slog.String("k", "a")}).(*prettyHandler)
	b, _ := parent.WithAttrs([]slog.Attr{slog.String("k", "b")}).(*prettyHandler)
	ga, _ := parent.WithGroup("a").(*prettyHandler)
	gb, _ := parent.WithGroup("b").(*prettyHandler)

	// Assert
	assert.Equal(t, "a", a.attrs[1].Value.String())
	assert.Equal(t, "b", b.attrs[1].Value.String())
	assert.Equal(t, []string{"p", "a"}, ga.groups)
	assert.Equal(t, []string{"p", "b"}, gb.groups)
}

func TestPrettyHandlerSiblingsWriteWholeLinesConcurrently(t *testing.T) {
	// Arrange
	var buf bytes.Buffer
	base := New(&buf, slog.LevelInfo)

	// Act
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			child := base.With("worker", i)
			for range 50 {
				child.Info("line")
			}
		}()
	}
	wg.Wait()

	// Assert
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	require.Len(t, lines, 400)
	for _, l := range lines {
		assert.Regexp(t, `^INFO +line worker=\d$`, ansiRe.ReplaceAllString(l, ""))
	}
}

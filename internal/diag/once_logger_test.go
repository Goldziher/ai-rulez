package diag

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/v5/internal/logger"
)

type warnLog struct{ warns []string }

func (w *warnLog) Debug(string, ...any)      {}
func (w *warnLog) Info(string, ...any)       {}
func (w *warnLog) Error(string, ...any)      {}
func (w *warnLog) Warn(msg string, _ ...any) { w.warns = append(w.warns, msg) }

// passThrough is a wrapper of another package that exposes what it wraps.
type passThrough struct{ logger.Logger }

func (p passThrough) Unwrap() logger.Logger { return p.Logger }

// A reload that hands the first load's host and collector on wraps the logger
// a second time; that must not drop the warnings the first wrapper says.
func TestOnceLoggerWrappingTwiceStillSaysEachWarningOnce(t *testing.T) {
	tests := []struct {
		name string
		wrap func(c *Collector, inner logger.Logger) logger.Logger
		want []string
	}{
		{name: "wrapped once", wrap: OnceLogger, want: []string{"a", "b"}},
		{name: "wrapped twice", wrap: func(c *Collector, inner logger.Logger) logger.Logger {
			return OnceLogger(c, OnceLogger(c, inner))
		}, want: []string{"a", "b"}},
		{name: "wrapped twice through another wrapper", wrap: func(c *Collector, inner logger.Logger) logger.Logger {
			return OnceLogger(c, passThrough{OnceLogger(c, inner)})
		}, want: []string{"a", "b"}},
		{name: "another collector wraps again", wrap: func(c *Collector, inner logger.Logger) logger.Logger {
			return OnceLogger(New(nil), OnceLogger(c, inner))
		}, want: []string{"a", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			out := &warnLog{}
			l := tt.wrap(New(nil), out)

			// Act
			for _, msg := range []string{"a", "b", "a", "b"} {
				l.Warn(msg)
			}

			// Assert
			assert.Equal(t, tt.want, out.warns)
		})
	}
}

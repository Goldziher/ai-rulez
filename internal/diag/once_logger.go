package diag

import (
	"fmt"

	"github.com/Goldziher/ai-rulez/v5/internal/logger"
)

// OnceLogger wraps inner so that a warning is issued once for the life of c: the
// same message with the same arguments, raised again, is dropped. A long-running
// loop that loads the same project on every cycle (watch, the MCP live reload)
// uses it so the warnings about the project are not repeated each cycle. A nil
// inner is the standard logger; the other levels pass through.
//
// Wrapping a logger that already says each warning once for c returns it as it
// is: the second wrapper would mark every warning seen before the first one
// could say it, and drop them all. A reload of a project that passes the first
// load's host and collector on (config.ReloadOptions) does exactly that. The
// check looks through wrappers that expose Unwrap() logger.Logger.
func OnceLogger(c *Collector, inner logger.Logger) logger.Logger {
	if saysOnceFor(c, inner) {
		return inner
	}
	return onceLogger{Logger: logger.Or(inner), c: c}
}

// saysOnceFor reports whether l, or a logger it wraps, is a OnceLogger of c.
func saysOnceFor(c *Collector, l logger.Logger) bool {
	for l != nil {
		if o, ok := l.(onceLogger); ok && o.c == c {
			return true
		}
		u, ok := l.(interface{ Unwrap() logger.Logger })
		if !ok {
			return false
		}
		l = u.Unwrap()
	}
	return false
}

type onceLogger struct {
	logger.Logger
	c *Collector
}

// Unwrap returns the wrapped logger.
func (l onceLogger) Unwrap() logger.Logger { return l.Logger }

func (l onceLogger) Warn(msg string, args ...any) {
	if l.c.Sticky("warn\x00" + msg + "\x00" + fmt.Sprint(args...)) {
		l.Logger.Warn(msg, args...)
	}
}

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
func OnceLogger(c *Collector, inner logger.Logger) logger.Logger {
	return onceLogger{Logger: logger.Or(inner), c: c}
}

type onceLogger struct {
	logger.Logger
	c *Collector
}

func (l onceLogger) Warn(msg string, args ...any) {
	if l.c.Sticky("warn\x00" + msg + "\x00" + fmt.Sprint(args...)) {
		l.Logger.Warn(msg, args...)
	}
}

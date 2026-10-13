package commands

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"

	"github.com/Goldziher/ai-rulez/v5/internal/logger"
)

// rootNote is one line a root's work wanted to say: a log record or, when raw,
// a line for the error stream.
type rootNote struct {
	level slog.Level
	raw   bool
	msg   string
	args  []any
}

// rootNotes collects what one root of a parallel run says while it works, so the
// run can print it in root order instead of in worker finish order. It satisfies
// logger.Logger, so a context carrying it (withRootNotes) routes the library's
// log records here too.
type rootNotes struct {
	mu    sync.Mutex
	items []rootNote
}

type rootNotesKey struct{}

// withRootNotes returns ctx carrying a fresh collector as both the root's notes
// and its logger.
func withRootNotes(ctx context.Context) (context.Context, *rootNotes) {
	n := &rootNotes{}
	return logger.WithContext(context.WithValue(ctx, rootNotesKey{}, n), n), n
}

func rootNotesFrom(ctx context.Context) *rootNotes {
	if ctx == nil {
		return nil
	}
	n, ok := ctx.Value(rootNotesKey{}).(*rootNotes)
	if !ok {
		return nil
	}
	return n
}

func (n *rootNotes) add(note rootNote) {
	n.mu.Lock()
	n.items = append(n.items, note)
	n.mu.Unlock()
}

func (n *rootNotes) Debug(msg string, args ...any) {
	n.add(rootNote{level: slog.LevelDebug, msg: msg, args: args})
}
func (n *rootNotes) Info(msg string, args ...any) {
	n.add(rootNote{level: slog.LevelInfo, msg: msg, args: args})
}
func (n *rootNotes) Warn(msg string, args ...any) {
	n.add(rootNote{level: slog.LevelWarn, msg: msg, args: args})
}
func (n *rootNotes) Error(msg string, args ...any) {
	n.add(rootNote{level: slog.LevelError, msg: msg, args: args})
}

// flush prints the collected notes, in the order they were recorded: log records
// to log, raw lines to w. A nil receiver prints nothing.
func (n *rootNotes) flush(log logger.Logger, w io.Writer) {
	if n == nil {
		return
	}
	n.mu.Lock()
	items := n.items
	n.items = nil
	n.mu.Unlock()
	for _, it := range items {
		switch {
		case it.raw:
			fmt.Fprint(w, it.msg) //nolint:errcheck // a failed diagnostic write has nowhere to be reported
		case it.level >= slog.LevelError:
			log.Error(it.msg, it.args...)
		case it.level >= slog.LevelWarn:
			log.Warn(it.msg, it.args...)
		case it.level >= slog.LevelInfo:
			log.Info(it.msg, it.args...)
		default:
			log.Debug(it.msg, it.args...)
		}
	}
}

// noteLine writes a raw line to w, or, when ctx belongs to a parallel root,
// holds it for that root's turn to print.
func noteLine(ctx context.Context, w io.Writer, line string) {
	if n := rootNotesFrom(ctx); n != nil {
		n.add(rootNote{raw: true, msg: line})
		return
	}
	fmt.Fprint(w, line) //nolint:errcheck // a failed diagnostic write has nowhere to be reported
}

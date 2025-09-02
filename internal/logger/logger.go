package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/samber/oops"
)

var (
	instance *slog.Logger
	once     sync.Once

	defaultLevel = slog.LevelInfo
	colorOutput  = true
	jsonOutput   = false
)

const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorPurple = "\033[35m"
	colorCyan   = "\033[36m"
	colorGray   = "\033[90m"
)

// Get returns the global logger instance
func Get() *slog.Logger {
	once.Do(func() {
		instance = New(os.Stderr, defaultLevel)
	})
	return instance
}

// New creates a new logger with the specified writer and level
func New(w io.Writer, level slog.Level) *slog.Logger {
	var handler slog.Handler

	if jsonOutput {
		handler = slog.NewJSONHandler(w, &slog.HandlerOptions{
			Level: level,
			ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
				if a.Key == slog.TimeKey {
					return slog.String("timestamp", a.Value.Time().Format(time.RFC3339))
				}
				return a
			},
		})
	} else {
		handler = &prettyHandler{
			w:           w,
			level:       level,
			colorOutput: colorOutput,
		}
	}

	return slog.New(handler)
}

// SetLevel sets the global log level
func SetLevel(level slog.Level) {
	defaultLevel = level
	instance = New(os.Stderr, level)
}

// SetColorOutput enables or disables colored output
func SetColorOutput(enabled bool) {
	colorOutput = enabled
	instance = New(os.Stderr, defaultLevel)
}

// SetJSONOutput enables or disables JSON output
func SetJSONOutput(enabled bool) {
	jsonOutput = enabled
	instance = New(os.Stderr, defaultLevel)
}

// prettyHandler implements a CLI-friendly slog handler
type prettyHandler struct {
	w           io.Writer
	level       slog.Level
	colorOutput bool
	mu          sync.Mutex
	attrs       []slog.Attr
	groups      []string
}

func (h *prettyHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

//nolint:gocritic // slog.Record parameter size is determined by standard library
func (h *prettyHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	var output strings.Builder

	if h.level <= slog.LevelDebug {
		timestamp := r.Time.Format("15:04:05")
		if h.colorOutput {
			output.WriteString(colorGray)
		}
		output.WriteString(timestamp)
		output.WriteString(" ")
		if h.colorOutput {
			output.WriteString(colorReset)
		}
	}

	levelStr, levelColor := h.formatLevel(r.Level)
	if h.colorOutput {
		output.WriteString(levelColor)
	}
	output.WriteString(levelStr)
	if h.colorOutput {
		output.WriteString(colorReset)
	}
	output.WriteString(" ")

	output.WriteString(r.Message)

	attrs := make([]slog.Attr, 0, r.NumAttrs())
	r.Attrs(func(a slog.Attr) bool {
		attrs = append(attrs, a)
		return true
	})

	allAttrs := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(allAttrs, h.attrs)
	copy(allAttrs[len(h.attrs):], attrs)

	if len(allAttrs) > 0 {
		output.WriteString(" ")
		if h.colorOutput {
			output.WriteString(colorGray)
		}

		parts := make([]string, 0, len(allAttrs))
		for _, attr := range allAttrs {
			parts = append(parts, h.formatAttr(attr))
		}
		output.WriteString(strings.Join(parts, " "))

		if h.colorOutput {
			output.WriteString(colorReset)
		}
	}

	output.WriteString("\n")

	_, err := h.w.Write([]byte(output.String()))
	return err
}

func (h *prettyHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &prettyHandler{
		w:           h.w,
		level:       h.level,
		colorOutput: h.colorOutput,
		attrs:       append(h.attrs, attrs...),
		groups:      h.groups,
	}
}

func (h *prettyHandler) WithGroup(name string) slog.Handler {
	return &prettyHandler{
		w:           h.w,
		level:       h.level,
		colorOutput: h.colorOutput,
		attrs:       h.attrs,
		groups:      append(h.groups, name),
	}
}

func (h *prettyHandler) formatLevel(level slog.Level) (levelStr, levelColor string) {
	switch level {
	case slog.LevelDebug:
		return "DEBUG", colorGray
	case slog.LevelInfo:
		return "INFO ", colorGreen
	case slog.LevelWarn:
		return "WARN ", colorYellow
	case slog.LevelError:
		return "ERROR", colorRed
	default:
		return "UNKN ", colorPurple
	}
}

func (h *prettyHandler) formatAttr(attr slog.Attr) string {
	if attr.Key == "error" {
		if err, ok := attr.Value.Any().(error); ok {
			return h.formatError(err)
		}
	}

	switch attr.Key {
	case "path", "file":
		if h.colorOutput {
			return fmt.Sprintf("%s%s=%v%s", colorCyan, attr.Key, attr.Value, colorReset)
		}
		return fmt.Sprintf("%s=%v", attr.Key, attr.Value)
	case "duration", "elapsed":
		if h.colorOutput {
			return fmt.Sprintf("%s%s=%v%s", colorBlue, attr.Key, attr.Value, colorReset)
		}
		return fmt.Sprintf("%s=%v", attr.Key, attr.Value)
	default:
		return fmt.Sprintf("%s=%v", attr.Key, attr.Value)
	}
}

func (h *prettyHandler) formatError(err error) string {
	var result strings.Builder

	if oopsErr, ok := oops.AsOops(err); ok {
		result.WriteString("error=")
		if h.colorOutput {
			result.WriteString(colorRed)
		}
		result.WriteString(err.Error())
		if h.colorOutput {
			result.WriteString(colorReset)
		}

		if hint := oopsErr.Hint(); hint != "" {
			result.WriteString(" hint=")
			if h.colorOutput {
				result.WriteString(colorCyan)
			}
			result.WriteString(hint)
			if h.colorOutput {
				result.WriteString(colorReset)
			}
		}
	} else {
		result.WriteString("error=")
		if h.colorOutput {
			result.WriteString(colorRed)
		}
		result.WriteString(err.Error())
		if h.colorOutput {
			result.WriteString(colorReset)
		}
	}

	return result.String()
}

// Convenience functions using the global logger

func Debug(msg string, args ...any) {
	Get().Debug(msg, args...)
}

func Info(msg string, args ...any) {
	Get().Info(msg, args...)
}

func Success(msg string, args ...any) {
	Get().Info(msg, args...)
}

func Warn(msg string, args ...any) {
	Get().Warn(msg, args...)
}

func Error(msg string, args ...any) {
	Get().Error(msg, args...)
}

// LogError logs an error with appropriate context
func LogError(msg string, err error, args ...any) {
	allArgs := append([]any{"error", err}, args...)
	Get().Error(msg, allArgs...)
}

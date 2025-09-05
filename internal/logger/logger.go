package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"

	"github.com/samber/oops"
)

var (
	instance *slog.Logger
	once     sync.Once

	defaultLevel = slog.LevelInfo
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

func Get() *slog.Logger {
	once.Do(func() {
		instance = New(os.Stderr, defaultLevel)
	})
	return instance
}

func New(w io.Writer, level slog.Level) *slog.Logger {
	handler := &prettyHandler{
		w:     w,
		level: level,
	}
	return slog.New(handler)
}

type prettyHandler struct {
	w      io.Writer
	level  slog.Level
	mu     sync.Mutex
	attrs  []slog.Attr
	groups []string
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
		output.WriteString(colorGray)
		output.WriteString(timestamp)
		output.WriteString(" ")
		output.WriteString(colorReset)
	}

	levelStr, levelColor := h.formatLevel(r.Level)
	output.WriteString(levelColor)
	output.WriteString(levelStr)
	output.WriteString(colorReset)
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
		output.WriteString(colorGray)

		parts := make([]string, 0, len(allAttrs))
		for _, attr := range allAttrs {
			parts = append(parts, h.formatAttr(attr))
		}
		output.WriteString(strings.Join(parts, " "))
		output.WriteString(colorReset)
	}

	output.WriteString("\n")

	_, err := h.w.Write([]byte(output.String()))
	return err
}

func (h *prettyHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &prettyHandler{
		w:      h.w,
		level:  h.level,
		attrs:  append(h.attrs, attrs...),
		groups: h.groups,
	}
}

func (h *prettyHandler) WithGroup(name string) slog.Handler {
	return &prettyHandler{
		w:      h.w,
		level:  h.level,
		attrs:  h.attrs,
		groups: append(h.groups, name),
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
		return fmt.Sprintf("%s%s=%v%s", colorCyan, attr.Key, attr.Value, colorReset)
	case "duration", "elapsed":
		return fmt.Sprintf("%s%s=%v%s", colorBlue, attr.Key, attr.Value, colorReset)
	default:
		return fmt.Sprintf("%s=%v", attr.Key, attr.Value)
	}
}

func (h *prettyHandler) formatError(err error) string {
	var result strings.Builder

	result.WriteString("error=")
	result.WriteString(colorRed)
	result.WriteString(err.Error())
	result.WriteString(colorReset)

	if oopsErr, ok := oops.AsOops(err); ok {
		if hint := oopsErr.Hint(); hint != "" {
			result.WriteString(" hint=")
			result.WriteString(colorCyan)
			result.WriteString(hint)
			result.WriteString(colorReset)
		}
	}

	return result.String()
}

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

func LogError(msg string, err error, args ...any) {
	allArgs := append([]any{"error", err}, args...)
	Get().Error(msg, allArgs...)
}

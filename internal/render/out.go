// Package render separates what a command produces from what it says about
// producing it. A command's result (a list, a plan, a report, a created path)
// goes to stdout in the requested format; progress, warnings, errors and hints
// go to stderr. -q removes only the progress and informational lines of the
// second stream: it never hides a result.
package render

import (
	"fmt"
	"io"
)

// Out is the pair of streams a command writes to.
type Out struct {
	stdout io.Writer
	stderr io.Writer
	quiet  bool
	format string
}

// New returns an Out over the two streams. quiet suppresses Info only.
func New(stdout, stderr io.Writer, quiet bool) Out {
	return Out{stdout: stdout, stderr: stderr, quiet: quiet}
}

// WithFormat returns o carrying the --format the command was asked for.
func (o Out) WithFormat(format string) Out {
	o.format = format
	return o
}

// JSON reports whether the command was asked for --format json.
func (o Out) JSON() bool { return o.format == "json" }

// Stdout is the stream results are written to.
func (o Out) Stdout() io.Writer { return o.stdout }

// Stderr is the stream diagnostics are written to.
func (o Out) Stderr() io.Writer { return o.stderr }

// Quiet reports whether -q is in effect.
func (o Out) Quiet() bool { return o.quiet }

// Result writes a formatted part of the command's result to stdout. It is never
// suppressed.
func (o Out) Result(format string, args ...any) {
	write(o.stdout, format, args...)
}

// Resultln writes a line of the command's result to stdout.
func (o Out) Resultln(args ...any) {
	_, _ = fmt.Fprintln(o.stdout, args...) //nolint:errcheck // nowhere to report a closed stdout
}

// Info writes a progress or confirmation line to stderr unless -q is set.
func (o Out) Info(format string, args ...any) {
	if o.quiet {
		return
	}
	write(o.stderr, format, args...)
}

// Warn writes a warning to stderr; -q does not hide it.
func (o Out) Warn(format string, args ...any) {
	write(o.stderr, format, args...)
}

// write prints to a stream that has nowhere to report its own failure.
func write(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...) //nolint:errcheck // nowhere to report a closed stream
}

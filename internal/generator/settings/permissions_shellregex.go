package settings

import (
	"regexp"
	"strings"
)

// Harnesses that match a whole-command regular expression (Augment, Zed) see the
// command as one string, so `git log; rm -rf /` satisfies `^git log(\s|$)`. An
// allow regex therefore only admits arguments free of shell control characters,
// and a deny regex is written so that a command chained after another is still
// caught. The classes use only syntax that JavaScript, Rust and Go read alike.

// shellControl lists what an allowed command may not contain: command
// separators, substitution, grouping and redirection.
const shellControl = "[^;&|\\n\\r`$()<>]"

// allowShellRegex is the regex of an allow rule: the command or prefix followed by
// plain arguments only. The separator after a prefix is a space or tab (not \s,
// which would admit a newline).
func allowShellRegex(p ShellPattern) string {
	q := regexp.QuoteMeta(p.Literal)
	switch p.Kind {
	case ShellPrefix:
		return "^" + q + "([ \\t]" + shellControl + "*)?$"
	case ShellExact:
		return "^" + q + "$"
	}
	return "^" + strings.ReplaceAll(q, `\*`, shellControl+"*") + "$"
}

// denyBoundary ends a denied command word: whitespace, a separator, a closing
// parenthesis or a backtick, or the end of the command.
const denyBoundary = "([\\s;&|)`]|$)"

// denyShellRegex is the regex of a deny rule, as broad as can be written without
// matching a different command word: a deny that is too wide only blocks more.
// anchored keeps the match at the start of the command, for a harness that also
// tests each chained sub-command (Zed); otherwise the command may follow a
// separator, a subshell or a backtick.
func denyShellRegex(p ShellPattern, anchored bool) string {
	q := regexp.QuoteMeta(p.Literal)
	start := "(^|[\\s;&|(`])"
	if anchored {
		start = "^"
	}
	switch p.Kind {
	case ShellPrefix:
		return start + q + denyBoundary
	case ShellExact:
		return "^" + q + "$"
	}
	return "(^|[\\s;&|(`])" + strings.ReplaceAll(q, `\*`, ".*")
}

package judge

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Goldziher/ai-rulez/v5/internal/llm"
)

// DefaultMaxFileBytes is how much of one file is sent: the head and the tail.
const DefaultMaxFileBytes = 32 * 1024

// Why a file is not sent.
const (
	SkipSecrets = "secrets"
	SkipEmpty   = "empty"
	SkipBinary  = "binary"
	// SkipUnreadable is for a file the caller could not read safely: a symlink,
	// a file that is not regular, or one over the size limit.
	SkipUnreadable = "unreadable"
)

// PrepareOptions tunes Prepare.
type PrepareOptions struct {
	// MaxBytes bounds the text sent; a longer file keeps its head and tail.
	MaxBytes int
	// Redact masks secret-looking text and sends the file; without it a file
	// that holds any is not sent.
	Redact bool
}

// Prepared is one file, ready to send or the reason it is not.
type Prepared struct {
	// Path is the slash path shown in reports.
	Path string
	// Content is the text to send; empty when Skip is set.
	Content string
	// Bytes is the size of the file.
	Bytes int
	// Truncated is set when the middle of the file was left out.
	Truncated bool
	// Redacted is set when secret-looking text was masked.
	Redacted bool
	// Skip is one of the Skip constants when the file is not sent.
	Skip       string
	SkipReason string
}

// Prepare screens and bounds data before it can be sent: binary or empty files
// and files that hold secret-looking text are skipped (unless Redact masks the
// text), and a long file keeps its head and tail. The check runs on the whole
// file, so a secret in the part left out still stops it.
func Prepare(path string, data []byte, opts PrepareOptions) Prepared {
	p := Prepared{Path: path, Bytes: len(data)}
	switch {
	case bytes.IndexByte(data, 0) >= 0 || !utf8.Valid(data):
		p.Skip, p.SkipReason = SkipBinary, "not UTF-8 text"
		return p
	case strings.TrimSpace(string(data)) == "":
		p.Skip, p.SkipReason = SkipEmpty, "the file is empty"
		return p
	}
	text := string(data)
	if redacted := llm.RedactSecrets(text); redacted != text {
		if !opts.Redact {
			p.Skip, p.SkipReason = SkipSecrets, "secret-looking text found; not sent (remove it, or pass --redact to send it masked)"
			return p
		}
		text, p.Redacted = redacted, true
	}
	maxBytes := opts.MaxBytes
	if maxBytes <= 0 {
		maxBytes = DefaultMaxFileBytes
	}
	p.Content, p.Truncated = truncate(text, maxBytes)
	return p
}

// truncate keeps the head (two thirds) and the tail (one third) of text when it
// is longer than maxBytes, cut on rune boundaries, with a marker between them.
func truncate(text string, maxBytes int) (string, bool) {
	if len(text) <= maxBytes {
		return text, false
	}
	headLen := maxBytes * 2 / 3
	tailLen := maxBytes - headLen
	head := text[:headLen]
	for head != "" && !utf8.ValidString(head) {
		head = head[:len(head)-1]
	}
	tail := text[len(text)-tailLen:]
	for tail != "" && !utf8.ValidString(tail) {
		tail = tail[1:]
	}
	omitted := len(text) - len(head) - len(tail)
	return head + "\n[... " + strconv.Itoa(omitted) + " bytes omitted from the middle ...]\n" + tail, true
}

// cleanText makes model output safe to print: secret-looking text is masked and
// control characters (terminal escapes, line breaks) become spaces.
func cleanText(s string) string {
	s = llm.RedactSecrets(s)
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || !unicode.IsPrint(r) && r != ' ' {
			return ' '
		}
		return r
	}, s)
	return strings.Join(strings.Fields(s), " ")
}

func quote(s string) string { return fmt.Sprintf("%q", s) }

func itoa(n int) string { return strconv.Itoa(n) }

// Package sarifout holds the pieces of SARIF 2.1.0 output that the lint and
// verifier writers must render identically: the document constants, how a path
// becomes an artifact URI, and how a severity becomes a result level.
package sarifout

import (
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
)

// Constants shared by every ai-rulez SARIF document.
const (
	Schema      = "https://json.schemastore.org/sarif-2.1.0.json"
	Version     = "2.1.0"
	SrcRoot     = "%SRCROOT%"
	InformURI   = "https://github.com/Goldziher/ai-rulez"
	LevelError  = "error"
	LevelWarn   = "warning"
	LevelNote   = "note"
	severityErr = "error"
	severityWrn = "warning"
)

var schemeRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*:`)

// ArtifactURI renders a path as a SARIF artifact URI reference. A
// repository-relative path becomes a slash-separated, percent-escaped reference
// under base SrcRoot. An absolute path or one that starts with a URI scheme
// becomes a file URI with no base.
func ArtifactURI(p string) (uri, base string) {
	p = filepath.ToSlash(p)
	// filepath.ToSlash is a no-op off Windows: normalise backslashes explicitly.
	p = strings.ReplaceAll(p, `\`, "/")
	if strings.HasPrefix(p, "/") || schemeRe.MatchString(p) {
		if !strings.HasPrefix(p, "/") {
			p = "/" + p // a drive letter is a path, not a host
		}
		return (&url.URL{Scheme: "file", Path: p}).String(), ""
	}
	parts := strings.Split(p, "/")
	for i, seg := range parts {
		parts[i] = url.PathEscape(seg)
	}
	return strings.Join(parts, "/"), SrcRoot
}

// Level maps a severity name ("error", "warning", anything else is a note) to a
// SARIF result level.
func Level(severity string) string {
	switch severity {
	case severityErr:
		return LevelError
	case severityWrn:
		return LevelWarn
	default:
		return LevelNote
	}
}

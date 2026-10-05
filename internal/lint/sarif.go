package lint

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// externalFinding is the JSON shape an external scanner may print, and the
// normalised form of a SARIF result.
type externalFinding struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Severity string `json:"severity"`
	Rule     string `json:"rule"`
	Message  string `json:"message"`
}

const (
	sarifVersion = "2.1.0"
	// maxScannerResults caps how many results one scanner may report; a scanner
	// past it is treated as broken instead of partially ingested.
	maxScannerResults = 10000
	// maxScannerMessage bounds a message in runes.
	maxScannerMessage = 500
)

type sarifText struct {
	Text     string `json:"text"`
	Markdown string `json:"markdown"`
}

type sarifLog struct {
	Version string     `json:"version"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Invocations []struct {
		ExecutionSuccessful *bool `json:"executionSuccessful"`
		Notifications       []struct {
			Level   string    `json:"level"`
			Message sarifText `json:"message"`
		} `json:"toolExecutionNotifications"`
	} `json:"invocations"`
	// OriginalURIBaseIDs maps a uriBaseId to the base it stands for.
	OriginalURIBaseIDs map[string]sarifBase `json:"originalUriBaseIds"`
	Results            []sarifResult        `json:"results"`
}

// sarifBase is one originalUriBaseIds entry; it may itself be relative to another base.
type sarifBase struct {
	URI       string `json:"uri"`
	URIBaseID string `json:"uriBaseId"`
}

type sarifResult struct {
	RuleID       string    `json:"ruleId"`
	Level        string    `json:"level"`
	Message      sarifText `json:"message"`
	Suppressions []struct {
		Kind string `json:"kind"`
	} `json:"suppressions"`
	Locations []struct {
		PhysicalLocation struct {
			ArtifactLocation struct {
				URI       string `json:"uri"`
				URIBaseID string `json:"uriBaseId"`
			} `json:"artifactLocation"`
			Region struct {
				StartLine int `json:"startLine"`
			} `json:"region"`
		} `json:"physicalLocation"`
	} `json:"locations"`
}

// parseExternal reads a scanner's stdout. A document that cannot be trusted
// (unreadable, a different SARIF version, a run that reports failure, more
// results than the cap) is an error: it is never ingested in part, and an
// empty document is never mistaken for a clean scan.
func parseExternal(format string, out []byte, exitCode int) ([]externalFinding, error) {
	if len(bytes.TrimSpace(out)) == 0 {
		return nil, fmt.Errorf("no output")
	}
	if strings.EqualFold(format, "json") {
		var list []externalFinding
		if err := json.Unmarshal(out, &list); err != nil {
			return nil, fmt.Errorf("invalid JSON: %w", err)
		}
		if len(list) > maxScannerResults {
			return nil, fmt.Errorf("more than %d results", maxScannerResults)
		}
		if len(list) == 0 && exitCode != 0 {
			return nil, fmt.Errorf("the scanner printed no results and exited with status %d", exitCode)
		}
		return list, nil
	}
	var log sarifLog
	if err := json.Unmarshal(out, &log); err != nil {
		return nil, fmt.Errorf("invalid SARIF: %w", err)
	}
	if log.Version != "" && log.Version != sarifVersion {
		return nil, fmt.Errorf("unsupported SARIF version %q (want %s)", log.Version, sarifVersion)
	}
	if len(log.Runs) == 0 && exitCode != 0 {
		return nil, fmt.Errorf("SARIF has no runs and the scanner exited with status %d", exitCode)
	}
	var found []externalFinding
	for _, run := range log.Runs {
		if err := sarifRunError(run); err != nil {
			return nil, err
		}
		for _, res := range run.Results {
			if sarifSuppressed(res) {
				continue
			}
			if len(found) >= maxScannerResults {
				return nil, fmt.Errorf("more than %d results", maxScannerResults)
			}
			msg := res.Message.Text
			if strings.TrimSpace(msg) == "" {
				msg = res.Message.Markdown
			}
			f := externalFinding{Rule: res.RuleID, Severity: res.Level, Message: msg}
			if len(res.Locations) > 0 {
				loc := res.Locations[0].PhysicalLocation
				f.File = sarifLocationPath(loc.ArtifactLocation.URI, loc.ArtifactLocation.URIBaseID, run.OriginalURIBaseIDs, 0)
				f.Line = loc.Region.StartLine
			}
			found = append(found, f)
		}
	}
	return found, nil
}

// maxSARIFBaseDepth bounds uriBaseId chains, so a cyclic declaration ends.
const maxSARIFBaseDepth = 8

// sarifLocationPath joins an artifact URI onto the base its uriBaseId declares.
// An absolute URI wins over the base; an id the log does not declare leaves the
// URI relative, which resolveScannerPath then anchors at the project root.
// Nothing here decides whether the result is acceptable: that is the caller's
// containment check, so a base pointing outside the project is rejected there.
func sarifLocationPath(uri, baseID string, bases map[string]sarifBase, depth int) string {
	if baseID == "" || depth > maxSARIFBaseDepth || strings.HasPrefix(uri, "/") || strings.Contains(uri, "://") {
		return uri
	}
	base, ok := bases[baseID]
	if !ok {
		return uri
	}
	prefix := sarifLocationPath(base.URI, base.URIBaseID, bases, depth+1)
	if prefix == "" {
		return uri
	}
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	return prefix + uri
}

// sarifRunError reports a run whose tool said it did not complete.
func sarifRunError(run sarifRun) error {
	for _, inv := range run.Invocations {
		if inv.ExecutionSuccessful != nil && !*inv.ExecutionSuccessful {
			return fmt.Errorf("the scanner reported executionSuccessful=false")
		}
		for _, n := range inv.Notifications {
			if strings.EqualFold(n.Level, "error") {
				return fmt.Errorf("the scanner reported an error: %s", sanitizeScannerText(n.Message.Text))
			}
		}
	}
	return nil
}

// sarifSuppressed reports a result the tool marked as suppressed in source or externally.
func sarifSuppressed(res sarifResult) bool {
	for _, s := range res.Suppressions {
		if k := strings.ToLower(s.Kind); k == "insource" || k == "external" {
			return true
		}
	}
	return false
}

var (
	ansiCSI = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)
	ansiOSC = regexp.MustCompile(`\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)`)
	winDriv = regexp.MustCompile(`^/[A-Za-z]:/`)
	// driveAuthority is a Windows drive letter in the authority slot of a file URI.
	driveAuthority = regexp.MustCompile(`^[A-Za-z]:$`)
)

// sanitizeScannerText makes attacker-influenced scanner text safe to print: no
// escape sequences, control, bidirectional, zero-width or tag characters, one
// line, at most maxScannerMessage runes, and secret-shaped substrings masked.
func sanitizeScannerText(s string) string {
	s = ansiOSC.ReplaceAllString(ansiCSI.ReplaceAllString(s, ""), "")
	var sb strings.Builder
	for _, r := range s {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			sb.WriteByte(' ')
		case unicode.IsControl(r) || isInvisibleRune(r):
		default:
			sb.WriteRune(r)
		}
	}
	out := maskSecrets(strings.Join(strings.Fields(sb.String()), " "))
	if utf8.RuneCountInString(out) > maxScannerMessage {
		out = string([]rune(out)[:maxScannerMessage]) + "..."
	}
	return out
}

func isInvisibleRune(r rune) bool {
	switch {
	case r >= 0x200B && r <= 0x200F, r >= 0x202A && r <= 0x202E, r >= 0x2060 && r <= 0x2064, r >= 0x2066 && r <= 0x2069:
		return true
	case r == 0xFEFF, r == 0x00AD, r >= 0xE0000 && r <= 0xE007F:
		return true
	}
	return false
}

// maskSecrets replaces every secret-shaped substring with its masked form, so a
// finding from a secret scanner does not carry the secret it found.
func maskSecrets(s string) string {
	for _, p := range builtinSecrets {
		s = p.re.ReplaceAllStringFunc(s, maskSecret)
	}
	return genericCredential.ReplaceAllStringFunc(s, func(m string) string {
		if sub := genericCredential.FindStringSubmatch(m); len(sub) > 1 && hasLetterAndDigit(sub[1]) {
			return strings.Replace(m, sub[1], maskSecret(sub[1]), 1)
		}
		return m
	})
}

// resolveScannerPath turns the path a scanner printed (a file:// URI, a percent
// encoded or Windows-style path, absolute or relative) into an absolute path
// inside root. ok is false when it points outside the project, so a scanner
// cannot attribute a finding to an arbitrary file.
func resolveScannerPath(raw, root string) (abs string, ok bool) {
	p := strings.TrimSpace(raw)
	if p == "" {
		return "", true
	}
	if rest, found := strings.CutPrefix(p, "file://"); found {
		if i := strings.IndexAny(rest, "?#"); i >= 0 {
			rest = rest[:i]
		}
		authority := rest
		path := ""
		if i := strings.IndexByte(rest, '/'); i >= 0 {
			authority, path = rest[:i], rest[i:]
		}
		switch {
		case authority == "" || strings.EqualFold(authority, hostLocalhost):
		case driveAuthority.MatchString(authority):
			// file://C:/x is a common malformed spelling of file:///C:/x
			path = "/" + authority + path
		default:
			// A UNC or remote host: not a file in this project.
			return "", false
		}
		p = path
		if winDriv.MatchString(p) {
			p = p[1:]
		}
	}
	if dec, err := url.PathUnescape(p); err == nil {
		p = dec
	}
	if os.PathSeparator == '\\' {
		p = strings.ReplaceAll(p, `\`, "/")
	}
	if strings.ContainsRune(p, 0) {
		return "", false
	}
	if !filepath.IsAbs(filepath.FromSlash(p)) {
		p = filepath.Join(root, filepath.FromSlash(p))
	}
	p = filepath.Clean(filepath.FromSlash(p))
	for _, base := range scannerRoots(root) {
		if rel, err := filepath.Rel(base, p); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return p, true
		}
	}
	return "", false
}

// scannerRoots is the root and its symlink-resolved form (a scanner may print
// either, as on macOS where /var is a link to /private/var).
func scannerRoots(root string) []string {
	roots := []string{filepath.Clean(root)}
	if real, err := filepath.EvalSymlinks(root); err == nil && real != roots[0] {
		roots = append(roots, real)
	}
	return roots
}

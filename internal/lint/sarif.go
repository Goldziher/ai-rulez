package lint

import (
	"bytes"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/samber/oops"
)

// externalFinding is the JSON shape an external scanner may print, and the
// normalised form of a SARIF result.
type externalFinding struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Severity string `json:"severity"`
	Rule     string `json:"rule"`
	Message  string `json:"message"`
	// Fingerprint is the scanner's own stable identity for the result (SARIF
	// partialFingerprints or fingerprints); empty when it gave none.
	Fingerprint string `json:"fingerprint,omitempty"`
	// The fields below come from SARIF only.
	// Score is properties.security-severity (result, else rule), valid when HasScore.
	Score    float64 `json:"-"`
	HasScore bool    `json:"-"`
	// DefaultLevel is the rule's defaultConfiguration.level.
	DefaultLevel string `json:"-"`
	// HelpURI is the rule's helpUri, an evidence link.
	HelpURI string `json:"-"`
	// Suppressed marks a result the tool suppressed in source or externally.
	Suppressed bool `json:"-"`
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
	Tool               struct {
		Driver struct {
			Rules []ingestRule `json:"rules"`
		} `json:"driver"`
	} `json:"tool"`
}

// ingestRule is the part of tool.driver.rules[] the ingest uses.
type ingestRule struct {
	ID                   string `json:"id"`
	HelpURI              string `json:"helpUri"`
	DefaultConfiguration struct {
		Level string `json:"level"`
	} `json:"defaultConfiguration"`
	Properties map[string]any `json:"properties"`
}

// sarifBase is one originalUriBaseIds entry; it may itself be relative to another base.
type sarifBase struct {
	URI       string `json:"uri"`
	URIBaseID string `json:"uriBaseId"`
}

type sarifResult struct {
	RuleID    string `json:"ruleId"`
	RuleIndex *int   `json:"ruleIndex"`
	Rule      struct {
		ID string `json:"id"`
	} `json:"rule"`
	Level               string         `json:"level"`
	Message             sarifText      `json:"message"`
	Properties          map[string]any `json:"properties"`
	Fingerprints        map[string]any `json:"fingerprints"`
	PartialFingerprints map[string]any `json:"partialFingerprints"`
	Suppressions        []struct {
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
	return parseExternalKeep(format, out, exitCode, false)
}

// parseExternalKeep is parseExternal that keeps suppressed results, flagged
// Suppressed, when keepSuppressed is set (--show-suppressed).
func parseExternalKeep(format string, out []byte, exitCode int, keepSuppressed bool) ([]externalFinding, error) {
	if len(bytes.TrimSpace(out)) == 0 {
		return nil, oops.Errorf("no output")
	}
	if name, ok := adapterName(format); ok {
		return parseAdapter(name, out, exitCode, keepSuppressed)
	}
	if strings.EqualFold(format, "json") {
		return parseJSONFindings(out, exitCode)
	}
	var log sarifLog
	if err := json.Unmarshal(out, &log); err != nil {
		return nil, oops.Wrapf(err, "invalid SARIF")
	}
	if log.Version != "" && log.Version != sarifVersion {
		return nil, oops.Errorf("unsupported SARIF version %q (want %s)", log.Version, sarifVersion)
	}
	if len(log.Runs) == 0 && exitCode != 0 {
		return nil, oops.Errorf("SARIF has no runs and the scanner exited with status %d", exitCode)
	}
	var found []externalFinding
	for _, run := range log.Runs {
		if err := sarifRunError(run); err != nil {
			return nil, err
		}
		for i := range run.Results {
			res := run.Results[i]
			suppressed := sarifSuppressed(res)
			if suppressed && !keepSuppressed {
				continue
			}
			if len(found) >= maxScannerResults {
				return nil, oops.Errorf("more than %d results", maxScannerResults)
			}
			found = append(found, sarifFinding(run, res, suppressed))
		}
	}
	return found, nil
}

// parseJSONFindings reads the scanner's own JSON list of findings.
func parseJSONFindings(out []byte, exitCode int) ([]externalFinding, error) {
	var list []externalFinding
	if err := json.Unmarshal(out, &list); err != nil {
		return nil, oops.Wrapf(err, "invalid JSON")
	}
	if len(list) > maxScannerResults {
		return nil, oops.Errorf("more than %d results", maxScannerResults)
	}
	if len(list) == 0 && exitCode != 0 {
		return nil, oops.Errorf("the scanner printed no results and exited with status %d", exitCode)
	}
	return list, nil
}

// sarifFinding converts one SARIF result of run into a finding: message, rule
// (from the result or its rules[] entry), score, fingerprint and first location.
func sarifFinding(run sarifRun, res sarifResult, suppressed bool) externalFinding {
	msg := res.Message.Text
	if strings.TrimSpace(msg) == "" {
		msg = res.Message.Markdown
	}
	f := externalFinding{Severity: res.Level, Message: msg, Suppressed: suppressed,
		Fingerprint: ingestFingerprint(res)}
	rule := ingestRuleOf(run, res)
	f.Rule = res.RuleID
	if f.Rule == "" {
		f.Rule = res.Rule.ID
	}
	if f.Rule == "" && rule != nil {
		f.Rule = rule.ID
	}
	if rule != nil {
		f.DefaultLevel, f.HelpURI = rule.DefaultConfiguration.Level, rule.HelpURI
	}
	f.Score, f.HasScore = ingestScore(res.Properties)
	if !f.HasScore && rule != nil {
		f.Score, f.HasScore = ingestScore(rule.Properties)
	}
	if len(res.Locations) > 0 {
		loc := res.Locations[0].PhysicalLocation
		f.File = sarifLocationPath(loc.ArtifactLocation.URI, loc.ArtifactLocation.URIBaseID, run.OriginalURIBaseIDs, 0)
		f.Line = min(max(loc.Region.StartLine, 0), maxScannerLine)
	}
	return f
}

// maxScannerLine bounds the line a scanner reports; a hostile value is clamped.
const maxScannerLine = 10_000_000

// ingestRuleOf finds the tool.driver.rules[] entry of a result: by ruleIndex, else by id.
func ingestRuleOf(run sarifRun, res sarifResult) *ingestRule {
	rules := run.Tool.Driver.Rules
	if i := res.RuleIndex; i != nil && *i >= 0 && *i < len(rules) {
		return &rules[*i]
	}
	id := res.RuleID
	if id == "" {
		id = res.Rule.ID
	}
	if id == "" {
		return nil
	}
	for i := range rules {
		if rules[i].ID == id {
			return &rules[i]
		}
	}
	return nil
}

// ingestScore reads properties["security-severity"], which is a number or a numeric string.
func ingestScore(props map[string]any) (float64, bool) {
	switch v := props["security-severity"].(type) {
	case float64:
		return v, true
	case string:
		if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			return f, true
		}
	}
	return 0, false
}

// maxScannerFingerprint bounds a scanner-provided fingerprint that is kept.
const maxScannerFingerprint = 512

// ingestFingerprint picks the scanner's own identity for a result: the
// primaryLocationLineHash partial fingerprint, else the first partial
// fingerprint by key, else the first fingerprint by key. Empty when none.
func ingestFingerprint(res sarifResult) string {
	pick := func(m map[string]any, preferred string) string {
		if v, ok := m[preferred].(string); ok && v != "" && len(v) <= maxScannerFingerprint {
			return v
		}
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if v, ok := m[k].(string); ok && v != "" && len(v) <= maxScannerFingerprint {
				return k + "=" + v
			}
		}
		return ""
	}
	if fp := pick(res.PartialFingerprints, "primaryLocationLineHash"); fp != "" {
		return fp
	}
	return pick(res.Fingerprints, "")
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
			return oops.Errorf("the scanner reported executionSuccessful=false")
		}
		for _, n := range inv.Notifications {
			if strings.EqualFold(n.Level, levelError) {
				return oops.Errorf("the scanner reported an error: %s", sanitizeScannerText(n.Message.Text))
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
	ansiCSI = regexp.MustCompile(`\x1b\[[0-9;?]*[\x20-\x2f]*[\x40-\x7e]`)
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
		var ok bool
		if p, ok = fileURLPath(rest); !ok {
			return "", false
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

// fileURLPath is the path of a file URL without its "file://" prefix; ok is
// false for a host other than this machine (a UNC or remote path).
func fileURLPath(rest string) (string, bool) {
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
		// the malformed spelling file://C:/x of the file URL for C:/x
		path = "/" + authority + path
	default:
		// A UNC or remote host: not a file in this project.
		return "", false
	}
	if winDriv.MatchString(path) {
		path = path[1:]
	}
	return path, true
}

// scannerRoots is the root and its symlink-resolved form (a scanner may print
// either, as on macOS where /var is a link to /private/var).
func scannerRoots(root string) []string {
	roots := []string{filepath.Clean(root)}
	if resolved, err := filepath.EvalSymlinks(root); err == nil && resolved != roots[0] {
		roots = append(roots, resolved)
	}
	return roots
}

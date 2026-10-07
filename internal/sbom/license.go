package sbom

import (
	_ "embed" // the SPDX license id list
	"strings"
	"sync"
	"unicode"
)

// spdxLicenseList is the SPDX license and exception ids of the CycloneDX 1.6
// schema's enum (spdx.schema.json), one per line.
//
//go:embed spdx_licenses.txt
var spdxLicenseList string

const maxLicenseLen = 256

var (
	spdxOnce       sync.Once
	spdxLicenses   map[string]string // lower-case id -> canonical id
	spdxExceptions map[string]string
)

func loadSPDXIDs() {
	spdxLicenses, spdxExceptions = map[string]string{}, map[string]string{}
	for _, id := range strings.Fields(spdxLicenseList) {
		lower := strings.ToLower(id)
		if strings.Contains(lower, "exception") || lower == "linux-syscall-note" || strings.HasSuffix(lower, "-note") {
			spdxExceptions[lower] = id
			continue
		}
		spdxLicenses[lower] = id
	}
}

// licenseKind classifies a declared license string.
type licenseKind int

const (
	licenseNone       licenseKind = iota
	licenseID                     // a single SPDX id
	licenseExpression             // an SPDX expression of ids with AND, OR, WITH and parentheses
	licenseName                   // anything else, kept as a free-form name
)

// classifyLicense normalises a frontmatter `license` value: an SPDX id is
// returned in its canonical spelling, an expression of SPDX ids with its
// operators upper-cased and spaced, anything else as a sanitized name.
func classifyLicense(raw string) (kind licenseKind, value string) {
	text := sanitizeText(raw, maxLicenseLen)
	if text == "" {
		return licenseNone, ""
	}
	spdxOnce.Do(loadSPDXIDs)
	if id, ok := spdxLicenses[strings.ToLower(text)]; ok {
		return licenseID, id
	}
	if expr, ok := normalizeExpression(text); ok {
		return licenseExpression, expr
	}
	return licenseName, text
}

// normalizeExpression parses an SPDX license expression made of known ids.
func normalizeExpression(text string) (string, bool) {
	spaced := strings.NewReplacer("(", " ( ", ")", " ) ").Replace(text)
	tokens := strings.Fields(spaced)
	if len(tokens) < 2 {
		return "", false
	}
	var out []string
	depth := 0
	expectOperand := true // an id or "(" is next
	afterWith := false    // an exception id is next
	for _, tok := range tokens {
		upper := strings.ToUpper(tok)
		switch {
		case afterWith:
			id, ok := spdxExceptions[strings.ToLower(tok)]
			if !ok {
				return "", false
			}
			out = append(out, id)
			afterWith = false
		case tok == "(":
			if !expectOperand {
				return "", false
			}
			depth++
			out = append(out, "(")
		case tok == ")":
			if expectOperand || depth == 0 {
				return "", false
			}
			depth--
			out = append(out, ")")
		case upper == "AND" || upper == "OR":
			if expectOperand {
				return "", false
			}
			out = append(out, upper)
			expectOperand = true
		case upper == "WITH":
			if expectOperand || len(out) == 0 {
				return "", false
			}
			out = append(out, upper)
			afterWith = true
		default:
			if !expectOperand {
				return "", false
			}
			plus := strings.HasSuffix(tok, "+")
			id, ok := spdxLicenses[strings.ToLower(strings.TrimSuffix(tok, "+"))]
			if !ok {
				return "", false
			}
			if plus {
				id += "+"
			}
			out = append(out, id)
			expectOperand = false
		}
	}
	if depth != 0 || expectOperand || afterWith {
		return "", false
	}
	joined := strings.Join(out, " ")
	return strings.NewReplacer("( ", "(", " )", ")").Replace(joined), true
}

// licenseChoices turns a declared license into CycloneDX licenses.
func licenseChoices(raw string) []LicenseChoice {
	switch kind, text := classifyLicense(raw); kind {
	case licenseID:
		return []LicenseChoice{{License: &License{ID: text}}}
	case licenseExpression:
		return []LicenseChoice{{Expression: text}}
	case licenseName:
		return []LicenseChoice{{License: &License{Name: text}}}
	}
	return nil
}

// sanitizeText makes frontmatter text safe to put in a document read by other
// tools: control, format and bidirectional characters are dropped, runs of white
// space collapse to one space and the result is capped at max bytes (on a rune
// boundary).
func sanitizeText(s string, maxLen int) string {
	var b strings.Builder
	space := false
	for _, r := range s {
		switch {
		case unicode.IsSpace(r):
			space = true
			continue
		case unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == unicode.ReplacementChar:
			continue
		}
		if space && b.Len() > 0 {
			b.WriteByte(' ')
		}
		space = false
		if b.Len()+len(string(r)) > maxLen {
			break
		}
		b.WriteRune(r)
	}
	return b.String()
}

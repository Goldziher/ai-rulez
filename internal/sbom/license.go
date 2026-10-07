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
	p := exprParser{expectOperand: true}
	for _, tok := range tokens {
		if !p.step(tok) {
			return "", false
		}
	}
	if p.depth != 0 || p.expectOperand || p.afterWith {
		return "", false
	}
	joined := strings.Join(p.out, " ")
	return strings.NewReplacer("( ", "(", " )", ")").Replace(joined), true
}

// exprParser is the state of normalizeExpression over the tokens so far.
type exprParser struct {
	out           []string
	depth         int
	expectOperand bool // an id or "(" is next
	afterWith     bool // an exception id is next
}

// step consumes one token; false means the expression is not valid.
func (p *exprParser) step(tok string) bool {
	upper := strings.ToUpper(tok)
	switch {
	case p.afterWith:
		id, ok := spdxExceptions[strings.ToLower(tok)]
		if !ok {
			return false
		}
		p.out = append(p.out, id)
		p.afterWith = false
	case tok == "(":
		if !p.expectOperand {
			return false
		}
		p.depth++
		p.out = append(p.out, "(")
	case tok == ")":
		if p.expectOperand || p.depth == 0 {
			return false
		}
		p.depth--
		p.out = append(p.out, ")")
	case upper == "AND" || upper == "OR":
		if p.expectOperand {
			return false
		}
		p.out = append(p.out, upper)
		p.expectOperand = true
	case upper == "WITH":
		if p.expectOperand || len(p.out) == 0 {
			return false
		}
		p.out = append(p.out, upper)
		p.afterWith = true
	default:
		return p.license(tok)
	}
	return true
}

// license consumes a license id, optionally with a trailing "+".
func (p *exprParser) license(tok string) bool {
	if !p.expectOperand {
		return false
	}
	id, ok := spdxLicenses[strings.ToLower(strings.TrimSuffix(tok, "+"))]
	if !ok {
		return false
	}
	if strings.HasSuffix(tok, "+") {
		id += "+"
	}
	p.out = append(p.out, id)
	p.expectOperand = false
	return true
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

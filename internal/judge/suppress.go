package judge

import (
	"fmt"
	"strings"
	"time"
)

// MaxSuppressionDays is the furthest an expiry may lie in the future: a suppression is a loan, not a
// permanent exemption.
const MaxSuppressionDays = 366

// Suppression silences one known finding. It is pinned to the quote: when the text changes, the
// suppression no longer matches and the finding returns. It expires, so a forgotten one cannot
// hide a finding forever.
type Suppression struct {
	Code    string
	Path    string
	Quote   string
	Reason  string
	Expires time.Time
}

// ParseSuppression validates the fields of a [[judge.suppress]] entry. expires is YYYY-MM-DD.
func ParseSuppression(code, path, quote, reason, expires string, now time.Time) (Suppression, error) {
	s := Suppression{Code: strings.ToUpper(strings.TrimSpace(code)), Path: strings.TrimSpace(path), Quote: strings.TrimSpace(quote), Reason: strings.TrimSpace(reason)}
	switch {
	case s.Code == "":
		return s, fmt.Errorf("a suppression needs a code")
	case func() bool { _, ok := LookupCode(s.Code); return !ok }():
		return s, fmt.Errorf("%q is not a finding code", s.Code)
	case s.Path == "":
		return s, fmt.Errorf("suppression %s needs a path", s.Code)
	case s.Quote == "":
		return s, fmt.Errorf("suppression %s %s needs the quote it is pinned to", s.Code, s.Path)
	case s.Reason == "":
		return s, fmt.Errorf("suppression %s %s needs a reason", s.Code, s.Path)
	}
	t, err := time.Parse("2006-01-02", strings.TrimSpace(expires))
	if err != nil {
		return s, fmt.Errorf("suppression %s %s needs an expires date as YYYY-MM-DD", s.Code, s.Path)
	}
	s.Expires = t.AddDate(0, 0, 1) // valid through the whole expiry day
	if s.Expires.After(now.AddDate(0, 0, MaxSuppressionDays)) {
		return s, fmt.Errorf("suppression %s %s expires more than %d days from now", s.Code, s.Path, MaxSuppressionDays)
	}
	return s, nil
}

// Expired reports whether the suppression no longer applies at now.
func (s Suppression) Expired(now time.Time) bool { return !now.Before(s.Expires) }

// Matches reports whether the suppression covers the finding: same code, same path (exact or a glob)
// and the pinned quote inside the finding's quote.
func (s *Suppression) Matches(f *Finding) bool {
	if s.Code != f.Code {
		return false
	}
	if s.Path != f.Location.Path {
		g, ok := CompileGlob(s.Path)
		if !ok || !g.Match(f.Location.Path) {
			return false
		}
	}
	want, have := NormalizeSpace(s.Quote), NormalizeSpace(f.Quote)
	return want != "" && strings.Contains(have, want)
}

// ApplySuppressions moves the findings a live suppression covers out of the list. It also returns the
// suppressions that have expired, so the caller can say so instead of silently resurrecting findings.
func ApplySuppressions(findings []*Finding, sups []Suppression, now time.Time) (kept, suppressed []*Finding, expired []Suppression) {
	for _, s := range sups {
		if s.Expired(now) {
			expired = append(expired, s)
		}
	}
	for _, f := range findings {
		var hit *Suppression
		for i := range sups {
			if !sups[i].Expired(now) && sups[i].Matches(f) {
				hit = &sups[i]
				break
			}
		}
		if hit == nil {
			kept = append(kept, f)
			continue
		}
		f.Suppressed = &SuppressionInfo{Reason: hit.Reason, Expires: hit.Expires.AddDate(0, 0, -1).Format("2006-01-02")}
		suppressed = append(suppressed, f)
	}
	return kept, suppressed, expired
}

package lint

import (
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
)

// ScanText runs the security family (AR0xx) over texts keyed by display name.
// It is for content that is about to be written, such as an import. An inline
// ai-rulez-lint-ignore comment inside the text is not honored: the author of the
// text must not be able to silence the check on their own content. lc supplies
// the [lint] settings (severities, allowed hosts, custom patterns); nil uses the
// defaults.
func ScanText(lc *config.LintConfig, texts map[string]string) []Finding {
	r := &runner{cfg: &config.Config{}, tree: &Tree{}, docs: map[string]doc{}}
	if lc != nil {
		r.lc = *lc
	}
	r.cwd = ""
	r.resolveSettings()
	names := make([]string, 0, len(texts))
	for name := range texts {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		r.docs[name] = doc{} // an empty doc has no ignore directives
		r.securityScan(name, strings.ToValidUTF8(texts[name], "�"))
	}
	sort.SliceStable(r.findings, func(i, j int) bool {
		a, b := r.findings[i], r.findings[j]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Code < b.Code
	})
	return r.findings
}

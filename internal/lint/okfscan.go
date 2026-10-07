package lint

import (
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/okfbridge"
)

// OKFScanner adapts the security scan to the pre-write hook okfbridge.Import
// runs over the text of an OKF bundle before it is converted. It is the same
// scan `import okf` uses, so an OKF include is refused the content a skill or a
// rule would be refused. lc selects the rule set; nil uses the defaults.
//
// It lives here, not in includes or okfbridge: lint already imports both
// transitively, while neither may import lint back.
func OKFScanner(lc *config.LintConfig) okfbridge.Scanner {
	return func(texts map[string]string) []okfbridge.SecurityFinding {
		var found []okfbridge.SecurityFinding
		for _, f := range ScanTexts(lc, texts) {
			found = append(found, okfbridge.SecurityFinding{
				Code:     f.Code,
				Severity: string(f.Severity),
				File:     f.File,
				Line:     f.Line,
				Message:  f.Message,
			})
		}
		return found
	}
}

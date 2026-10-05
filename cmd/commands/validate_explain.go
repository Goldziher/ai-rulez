package commands

import (
	"encoding/json"
	"io"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/samber/oops"
)

// validateExplain is the rule code or name `validate --explain` describes.
var validateExplain string

// runExplain prints the explanation of one rule. With --format json it prints
// the structured record instead.
func runExplain(w io.Writer, key, format string) error {
	e, ok := lint.Explain(key)
	if !ok {
		return oops.Hint("Run `ai-rulez validate --explain AR001`; codes are listed in docs/strict-validation.md").
			Errorf("unknown rule %q", key)
	}
	if format == formatJSON {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(e) //nolint:wrapcheck // writer error
	}
	var sb strings.Builder
	lint.WriteExplanation(&sb, e)
	_, err := io.WriteString(w, sb.String())
	return err //nolint:wrapcheck // writer error
}

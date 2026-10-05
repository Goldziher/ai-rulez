package providers

import (
	"fmt"
	"path/filepath"
	"unicode/utf8"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/presets"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/rulefiles"
	"github.com/samber/oops"
)

// checkItems supplies the items of an outputs.checks block: the deduplicated
// checks of the (already profile-selected) content tree. It is a variable so
// tests can substitute fixed items.
var checkItems = presets.AllChecks

// sanitizeCheckName limits a check name to [A-Za-z0-9._-] for use in the section
// marker, so no name can close the HTML comment ("-->") or break its line.
func sanitizeCheckName(name string) string { return presets.SanitizeCheckName(name) }

// checkSizeLimits are the documented character limits of an aggregate check file
// the tool truncates at, by provider: Kilo Code Reviews stops reading REVIEW.md at
// 10,000 characters.
var checkSizeLimits = map[string]int{"kilo": 10000}

// renderAggregate renders every item of an aggregate output into its single file:
//
//	<header>
//
//	<!-- ai-rulez:checks:begin -->
//	<!-- ai-rulez:check:NAME -->
//
//	## NAME
//
//	body
//	<!-- ai-rulez:checks:end -->
//
// The sections sit between the begin and end markers, and a file that exists
// already keeps everything outside them (see presets.MergedChecksFile). It
// returns nil when there are no items, so a project without checks gets no file.
func (g *Generator) renderAggregate(typ string, spec *OutputSpec, items []config.ContentFile, baseDir string, cfg *config.Config) (*config.OutputFile, error) {
	var allowed []config.ContentFile
	for _, item := range items {
		if g.filterAllows(spec, item) {
			allowed = append(allowed, item)
		}
	}
	text := presets.RenderCheckSections(allowed, "")
	if text == "" {
		return nil, nil
	}
	outputPath := filepath.Join(baseDir, filepath.FromSlash(spec.File))
	out, err := presets.MergedChecksFile(outputPath, spec.Header, text)
	if err != nil {
		return nil, oops.With("preset", g.Spec.Name, "path", outputPath).Wrap(err)
	}
	if limit, ok := checkSizeLimits[g.Spec.Name]; ok {
		if size := utf8.RuneCount(out.RawContent); size > limit {
			rulefiles.Warn(fmt.Sprintf("%s is %d characters, over the %d that %s truncates the file at, so trailing checks are not read",
				spec.File, size, limit, g.Spec.Name), "hint", "shorten or drop checks, or target some of them at other presets")
		}
	}
	cfg.Analysis.Begin(outputPath, g.Spec.Name, outputKindForType(typ), "", "").
		AddPart(config.PartKindItemBody, typ, "", text)
	return &out, nil
}

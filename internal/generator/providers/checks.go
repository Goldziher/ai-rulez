package providers

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
)

// checkItems is the extension point that supplies the items of an
// outputs.checks block. Until the checks content kind exists in the content tree
// it yields none, so a spec can already declare [outputs.checks] and render
// nothing; the content kind replaces this function when it lands.
var checkItems = func(_ *config.ContentTree) []config.ContentFile { return nil }

// checkSectionMarker opens one check section of an aggregate file. The marker
// names the check, so a tool (or a later run) can find its section.
const checkSectionMarker = "<!-- ai-rulez:check:%s -->"

var unsafeCheckNameChars = regexp.MustCompile(`[^A-Za-z0-9._-]`)

// sanitizeCheckName limits a check name to [A-Za-z0-9._-] for use in the section
// marker, so no name can close the HTML comment ("-->") or break its line.
func sanitizeCheckName(name string) string {
	if clean := unsafeCheckNameChars.ReplaceAllString(name, "-"); clean != "" {
		return clean
	}
	return "check"
}

// renderAggregate renders every item of an aggregate output into its single file:
//
//	<header>
//
//	<!-- ai-rulez:check:NAME -->
//
//	## NAME
//
//	body
//
// It returns nil when there are no items, so a project without checks gets no
// file.
func (g *Generator) renderAggregate(typ string, spec *OutputSpec, items []config.ContentFile, baseDir string, cfg *config.Config) *config.OutputFile {
	var sections []string
	for _, item := range items {
		if !g.filterAllows(spec, item) {
			continue
		}
		sections = append(sections, strings.Join([]string{
			strings.Replace(checkSectionMarker, "%s", sanitizeCheckName(item.Name), 1),
			"## " + item.Name,
			strings.TrimRight(item.Content, "\n"),
		}, "\n\n"))
	}
	if len(sections) == 0 {
		return nil
	}
	var b strings.Builder
	if header := strings.TrimRight(spec.Header, "\n"); header != "" {
		b.WriteString(header)
		b.WriteString("\n\n")
	}
	b.WriteString(strings.Join(sections, "\n\n"))
	b.WriteString("\n")

	outputPath := filepath.Join(baseDir, filepath.FromSlash(spec.File))
	cfg.Analysis.Begin(outputPath, g.Spec.Name, outputKindForType(typ), "", "").
		AddPart(config.PartKindItemBody, typ, "", b.String())
	return &config.OutputFile{Path: outputPath, Content: b.String()}
}

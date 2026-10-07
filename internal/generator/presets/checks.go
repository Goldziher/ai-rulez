package presets

import (
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/docmerge"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/targetmatch"
)

// ChecksBlockName names the marker-delimited block ai-rulez owns in a review-check
// file shared with its user: <!-- ai-rulez:checks:begin --> ... <!-- ai-rulez:checks:end -->.
const ChecksBlockName = "checks"

// MergedDocCursorBugbot is the Bugbot instruction file the cursor preset merges
// its checks into.
const MergedDocCursorBugbot = ".cursor/BUGBOT.md"

// CheckSectionMarker opens one check section of an aggregate review file. The
// marker names the check, so a tool (or a later run) can find its section.
const CheckSectionMarker = "<!-- ai-rulez:check:%s -->"

var unsafeCheckNameChars = regexp.MustCompile(`[^A-Za-z0-9._-]`)

// SanitizeCheckName limits a check name to [A-Za-z0-9._-] for use in the section
// marker, so no name can close the HTML comment ("-->") or break its line.
func SanitizeCheckName(name string) string {
	if clean := unsafeCheckNameChars.ReplaceAllString(name, "-"); clean != "" {
		return clean
	}
	return "check"
}

// getAllDomainChecks flattens the domains' checks in precedence order.
func getAllDomainChecks(content *config.ContentTree) []config.ContentFile {
	var checks []config.ContentFile
	for _, name := range domainNamesByPrecedence(content) {
		checks = append(checks, content.Domains[name].Checks...)
	}
	return checks
}

// AllChecks returns the checks of the root and every selected domain, root first
// where names collide, sorted by name. The tree is already profile-selected.
//
// Names are checked here, at render time, because includes can bring checks that
// never went through config validation, and a name reaches output paths and
// marker comments: one that is not [A-Za-z0-9._-] is skipped with a warning. Two
// names that differ only in case are one check (the output files collide on a
// case-insensitive file system); the first, by precedence, is kept and the other
// reported.
//
// Each warning is shown once per run: every preset renders the same checks, and a
// warning per preset would repeat itself.
func AllChecks(cfg *config.Config, content *config.ContentTree) []config.ContentFile {
	if content == nil {
		return nil
	}
	// A message is shown once per run, however many presets render the checks.
	warnOnce := func(msg string, kv ...any) { cfg.WarnOnce(msg, msg, kv...) }
	combined := append(append([]config.ContentFile(nil), content.Checks...), getAllDomainChecks(content)...)
	seen := make(map[string]string, len(combined))
	kept := make([]config.ContentFile, 0, len(combined))
	for idx := range combined {
		check := combined[idx]
		if !config.IsValidCheckName(check.Name) {
			warnOnce("Skipping a check with an invalid name; it is rendered into file names and markers, "+
				"so only letters, digits, '.', '_' and '-' are allowed", "name", check.Name, "path", check.Path)
			continue
		}
		key := strings.ToLower(check.Name)
		if first, dup := seen[key]; dup {
			if first != check.Name {
				warnOnce("Skipping check \""+check.Name+"\": its name differs only in case from \""+first+
					"\", and their files would collide on a case-insensitive file system", "path", check.Path)
			}
			continue
		}
		seen[key] = check.Name
		kept = append(kept, check)
	}
	sort.SliceStable(kept, func(i, j int) bool { return kept[i].Name < kept[j].Name })
	return kept
}

// ChecksForPreset keeps the checks whose `targets` select the preset (a check
// without targets applies to every preset).
func ChecksForPreset(checks []config.ContentFile, preset string) []config.ContentFile {
	var out []config.ContentFile
	for idx := range checks {
		check := checks[idx]
		if check.Metadata != nil && !targetmatch.Allow(check.Metadata.Targets, []string{preset}) {
			continue
		}
		out = append(out, check)
	}
	return out
}

// RenderCheckSections renders checks into one aggregate review document:
//
//	<header>
//
//	<!-- ai-rulez:check:NAME -->
//
//	## NAME
//
//	body
//
// The body is the check's text, or its description when the text is empty. It
// returns "" when there are no checks, so a project without checks gets no file.
func RenderCheckSections(checks []config.ContentFile, header string) string {
	sections := make([]string, 0, len(checks))
	for i := range checks {
		check := &checks[i]
		parts := []string{
			strings.Replace(CheckSectionMarker, "%s", SanitizeCheckName(check.Name), 1),
			"## " + SanitizeCheckName(check.Name),
		}
		body := strings.TrimRight(check.Content, "\n")
		if strings.TrimSpace(body) == "" {
			body = config.CheckDescription(check)
		}
		if strings.TrimSpace(body) != "" {
			parts = append(parts, body)
		}
		sections = append(sections, strings.Join(parts, "\n\n"))
	}
	if len(sections) == 0 {
		return ""
	}
	var b strings.Builder
	if header = strings.TrimRight(header, "\n"); header != "" {
		b.WriteString(header)
		b.WriteString("\n\n")
	}
	b.WriteString(strings.Join(sections, "\n\n"))
	b.WriteString("\n")
	return b.String()
}

// MergedChecksFile renders text, the check sections, into the review-check file at
// path. The text is a marker-delimited block: a file ai-rulez creates holds the
// header (when given) and the block, and a hand-written file keeps everything
// outside the markers (the block is appended when it has none). The file is
// partially owned when it holds anything else, so it is never deleted whole and
// never git-ignored. It is written verbatim (RawContent): a generated-file banner
// or content hash would land inside the user's own text.
func MergedChecksFile(cfg *config.Config, path, header, text string) (config.OutputFile, error) {
	res, err := docmerge.ApplyWith(cfg.ReadExisting, path, docmerge.FormatMarkdown, []docmerge.OwnedKey{
		{Name: docmerge.HeaderKey, Value: header},
		{Name: ChecksBlockName, Value: text},
	})
	if err != nil {
		return config.OutputFile{}, oops.With("path", path).Wrapf(err, "merge the review checks into the existing file")
	}
	return config.OutputFile{
		Path:           path,
		RawContent:     []byte(res.Body),
		PartiallyOwned: res.PartiallyOwned,
		MergeClaims:    res.Claims,
		Committed:      true,
	}, nil
}

// cursorChecksFile is Bugbot's repository-root review instruction file.
var cursorChecksFile = filepath.FromSlash(MergedDocCursorBugbot)

// cursorCheckOutputs renders the checks targeting Cursor into .cursor/BUGBOT.md
// (Bugbot reads one instruction file per directory, not one per check). User
// scope writes nothing: Bugbot reads the repository's file, and no per-user
// location is documented.
func cursorCheckOutputs(content *config.ContentTree, baseDir string, cfg *config.Config) ([]config.OutputFile, error) {
	if cfg != nil && cfg.UserScope {
		return nil, nil
	}
	text := RenderCheckSections(ChecksForPreset(AllChecks(cfg, content), presetNameCursor), "")
	if text == "" {
		return nil, nil
	}
	out, err := MergedChecksFile(cfg, filepath.Join(baseDir, cursorChecksFile), "", text)
	if err != nil {
		return nil, err
	}
	return []config.OutputFile{out}, nil
}

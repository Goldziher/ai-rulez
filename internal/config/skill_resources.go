package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
	"github.com/samber/oops"
)

// SkillResourceKind identifiers for the canonical Agent Skills layout.
const (
	SkillKindReferences = "references"
	SkillKindScripts    = "scripts"
	SkillKindAssets     = "assets"
)

// Author-facing noun for each kind of item that owns a directory. Interpolated
// into diagnostics, so a command directory is never described as a skill — an
// author sent to the skills documentation for a commands/ mistake learns nothing.
const (
	ItemKindSkill   = "skill"
	ItemKindCommand = "command"
)

// SkillKindEvals is the conventional directory for eval cases next to a skill.
// It is recognized (no warning) but never emitted into a per-tool skill tree,
// because cases must not cost context; `[plugin] include_evals` opts in to
// bundling them.
const SkillKindEvals = "evals"

// EvalsDirName is the name of the project-level eval tree under the config dir.
const EvalsDirName = "evals"

// skillResourceKinds is the ordered list of subdirectories the loader walks
// under a skill root. Order is meaningful: it determines the order
// resources appear in the rendered SKILL.md index.
var skillResourceKinds = []string{
	SkillKindReferences,
	SkillKindScripts,
	SkillKindAssets,
}

// LoadSkillResources loads the resources of a skill directory. Thin wrapper
// over LoadResources for the common case.
func LoadSkillResources(skillDir string) ([]SkillResource, error) {
	return LoadResources(skillDir, ItemKindSkill)
}

// LoadSkillResourcesContext is LoadSkillResources reporting through the logger
// ctx carries (logger.WithContext) and asking the VCS, to leave out ignored
// files, through the runner ctx carries.
func LoadSkillResourcesContext(ctx context.Context, skillDir string) ([]SkillResource, error) {
	s := newIncludeScanner(ctx, osView(skillDir))
	s.log = logger.FromContext(ctx)
	s.git = gitutil.New(runner.FromContext(ctx))
	return s.loadResources(skillDir, ItemKindSkill, nil)
}

// LoadResources is LoadResourcesWith with only the default bundle excludes.
func LoadResources(root, itemKind string) ([]SkillResource, error) {
	return LoadResourcesWith(root, itemKind, nil)
}

// LoadResources walks a skill or command directory and returns its supporting
// files from references/, scripts/, and assets/ subdirectories. Files are read
// as raw bytes so binary assets round-trip without UTF-8 corruption.
//
// For markdown references the function also extracts a short description,
// preferring the frontmatter `description` field, falling back to the first
// non-empty content line (with markdown heading markers stripped).
//
// Missing subdirectories are not an error — they simply contribute no
// resources. Walk failures within a subdirectory are returned to the caller
// so the loader can decide whether to surface or warn.
//
// Unrecognized subdirectories (any directory other than references/, scripts/,
// assets/ or evals/) trigger a warning naming the item and the offending directory.
// The Agent Skills spec defines only these three as canonical resource kinds.
//
// Build artifacts are not bundled: files git ignores (when root is in a git
// work tree), anything matching DefaultBundleExcludes (.venv*, venv,
// __pycache__, *.pyc, node_modules, .git) and anything matching extraExcludes
// (the bundle_exclude config key) are skipped.
//
// itemKind is one of the ItemKind constants and selects the entry file name
// (SKILL.md, COMMAND.md) and the diagnostics wording.
func LoadResourcesWith(root, itemKind string, extraExcludes []string) ([]SkillResource, error) {
	return newIncludeScanner(context.Background(), osView(root)).loadResources(root, itemKind, extraExcludes)
}

// loadResources is LoadResourcesWith under the scanner's symlink policy: with no
// project root (included and installed content) a symlink is never followed;
// for the project's own content a link is followed when its target stays inside
// the project, and refused, warned about and recorded otherwise.
func (s *contentScanner) loadResources(root, itemKind string, extraExcludes []string) ([]SkillResource, error) {
	marker := skillMarkerFile
	if itemKind == ItemKindCommand {
		marker = commandMarkerFile
	}
	filter := newBundleFilter(s.ctx, s.git, s.logger(), root, marker, extraExcludes)
	var resources []SkillResource

	for _, kind := range skillResourceKinds {
		kindDir := filepath.Join(root, kind)
		// A symlinked kind directory is admitted only under the scanner's
		// policy: an installed skill with `references -> /etc` must not let
		// the walk into an attacker-controlled tree.
		info, err := s.v.Lstat(kindDir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, oops.With("path", kindDir).Wrapf(err, "stat %s resource dir", itemKind)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			if _, ok := s.admit(kindDir); !ok {
				continue
			}
		}
		if info, err = s.v.Stat(kindDir); err != nil || !info.IsDir() {
			continue
		}

		kindResources, err := s.walkSkillResourceDir(root, kindDir, kind, filter)
		if err != nil {
			return nil, err
		}
		resources = append(resources, kindResources...)
	}

	// Warn about unrecognized subdirectories so authors know their content is
	// not being included in generated output. Only directories are checked;
	// regular files in the item root (like SKILL.md, .gitignore) are expected
	// and not warned about.
	warnings, err := s.unrecognizedSubdirectoryWarnings(root, itemKind)
	if err != nil {
		return nil, err
	}
	for _, warning := range warnings {
		// The kind doubles as the attribute key, so the line reads
		// skill=<path> or command=<path>.
		s.logger().Warn(
			warning.Message,
			warning.Kind, warning.Root,
			"subdirectory", warning.Subdirectory,
			"recognized_kinds", strings.Join(skillResourceKinds, ", "),
		)
	}

	return resources, nil
}

// resourceWarning is one author-facing advisory about content that will not
// reach the generated output. Carried as data rather than logged at the point of
// detection so a test can assert the exact wording the author is shown.
type resourceWarning struct {
	Kind         string
	Root         string
	Subdirectory string
	Message      string
}

// unrecognizedSubdirectoryWarnings returns one advisory per subdirectory that is
// not in the canonical set (references/, scripts/, assets/), sorted by name for
// a deterministic warning order. Content in such a directory is never walked, so
// naming it is the only way an author learns it was dropped.
//
// Returning the advisories rather than logging inline keeps the decision
// testable: a test can assert exactly which directories were flagged, and with
// which wording, without capturing the logger singleton, which has no injection
// seam.
//
// Regular files in the item root are ignored — only subdirectories are
// checked. Symlinked directories are ignored because the kind walk already
// refuses to follow them and warns separately.
func (s *contentScanner) unrecognizedSubdirectoryWarnings(root, itemKind string) ([]resourceWarning, error) {
	entries, err := s.v.ReadDir(root)
	if err != nil {
		return nil, oops.With("path", root).Wrapf(err, "read %s directory for unrecognized subdirs", itemKind)
	}

	recognized := make(map[string]bool, len(skillResourceKinds)+1)
	for _, kind := range skillResourceKinds {
		recognized[kind] = true
	}
	// evals/ is a known directory that is deliberately not emitted.
	recognized[SkillKindEvals] = true

	var warnings []resourceWarning
	for _, entry := range entries {
		// Regular files (SKILL.md, COMMAND.md, .gitignore, …) in the item root
		// are expected and are intentionally not resource kinds.
		if !entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			// A stat failure must not block the load, but it is still a
			// diagnostic the author needs.
			s.logger().Warn("Could not stat subdirectory", "owner", itemKind, "path", root, "name", entry.Name(), "error", err)
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			continue
		}

		if !recognized[entry.Name()] {
			warnings = append(warnings, resourceWarning{
				Kind:         itemKind,
				Root:         root,
				Subdirectory: entry.Name(),
				Message: fmt.Sprintf(
					"Unrecognized %s subdirectory will not be included in generated output", itemKind),
			})
		}
	}

	sort.Slice(warnings, func(i, j int) bool {
		return warnings[i].Subdirectory < warnings[j].Subdirectory
	})

	return warnings, nil
}

// walkSkillResourceDir walks one of the kind subdirectories recursively.
// Nested directories under references/, scripts/, or assets/ are preserved in
// the resource RelPath so generators can mirror the layout in their output.
// Symlinks are admitted entry by entry under the scanner's policy, so a link
// can neither read nor list anything outside what the policy allows.
func (s *contentScanner) walkSkillResourceDir(skillDir, kindDir, kind string, filter *bundleFilter) ([]SkillResource, error) {
	var resources []SkillResource
	visited := map[string]bool{}

	var visit func(dir string) error
	visit = func(dir string) error {
		if real, err := s.v.EvalSymlinks(dir); err == nil {
			if visited[real] {
				return nil // a link cycle inside the project
			}
			visited[real] = true
			defer delete(visited, real)
		}
		entries, err := s.v.ReadDir(dir)
		if err != nil {
			return oops.With("path", dir).Wrapf(err, "walk skill resource dir")
		}
		for _, d := range entries {
			path := filepath.Join(dir, d.Name())
			isDir, ok := s.entryInfo(path, d)
			if !ok {
				continue
			}
			if isDir {
				if filter.excluded(filter.rel(skillDir, path)) {
					continue
				}
				if err := visit(path); err != nil {
					return err
				}
				continue
			}
			if info, ok := s.admit(path); !ok || !info.Mode().IsRegular() {
				continue
			}

			// Path relative to the skill root, e.g. "references/api.md".
			relToSkill, err := filepath.Rel(skillDir, path)
			if err != nil {
				return oops.With("path", path).Wrapf(err, "compute relative path")
			}
			// Always use forward slashes in stored paths so output is portable across
			// platforms — generators feed this straight into filepath.Join, which
			// accepts forward slashes on Windows.
			relToSkill = filepath.ToSlash(relToSkill)
			if !filter.keepFile(relToSkill) {
				continue
			}

			data, err := readCapped(s.v, path)
			if err != nil {
				return oops.With("path", path).Wrapf(err, "read skill resource")
			}

			// Capture file mode (of the target, for a followed link) so the
			// executable bit on bundled scripts survives generation.
			info, err := s.v.Stat(path)
			if err != nil {
				return oops.With("path", path).Wrapf(err, "stat skill resource")
			}

			resource := SkillResource{Kind: kind, RelPath: relToSkill, Content: data, Mode: resourceMode(info.Mode())}
			if kind == SkillKindReferences && strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
				resource.Description = extractResourceDescription(data)
			}
			resources = append(resources, resource)
		}
		return nil
	}
	if err := visit(kindDir); err != nil {
		return nil, err
	}

	// Sort by RelPath so output ordering is deterministic across platforms.
	sort.Slice(resources, func(i, j int) bool {
		return resources[i].RelPath < resources[j].RelPath
	})

	return resources, nil
}

// extractResourceDescription pulls a one-line summary out of a reference
// markdown file. Preference order:
//  1. Frontmatter `description` field.
//  2. First non-empty, non-frontmatter line (stripped of leading `#` and
//     whitespace).
//
// Returns an empty string when neither source yields anything useful.
func extractResourceDescription(data []byte) string {
	return SummarizeResourceDescription(rawResourceDescription(data))
}

// MaxResourceDescriptionRunes bounds a derived resource description. A reference
// whose first line is a whole document would otherwise be inlined into the
// rendered SKILL.md index, inflating it past the serving size limit.
const MaxResourceDescriptionRunes = 160

// SummarizeResourceDescription reduces text to a single-line summary: control
// characters become spaces, whitespace collapses, the first sentence is kept
// and the result is cut at a rune boundary to MaxResourceDescriptionRunes.
func SummarizeResourceDescription(text string) string {
	// Bound the work on a pathological multi-megabyte line.
	if len(text) > 4*MaxResourceDescriptionRunes*utf8.UTFMax {
		text = text[:4*MaxResourceDescriptionRunes*utf8.UTFMax]
		for !utf8.ValidString(text) {
			text = text[:len(text)-1]
		}
	}
	text = strings.Join(strings.Fields(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, text)), " ")
	for i := 0; i+1 < len(text); i++ {
		if (text[i] == '.' || text[i] == '!' || text[i] == '?') && text[i+1] == ' ' {
			text = text[:i+1]
			break
		}
	}
	if utf8.RuneCountInString(text) <= MaxResourceDescriptionRunes {
		return text
	}
	runes := []rune(text)[:MaxResourceDescriptionRunes-1]
	return strings.TrimSpace(string(runes)) + "…"
}

func rawResourceDescription(data []byte) string {
	metadata, body := ParseFrontmatterPublic(string(data))
	if metadata != nil {
		if desc, ok := metadata.Extra["description"]; ok {
			desc = strings.TrimSpace(desc)
			if desc != "" {
				return desc
			}
		}
	}

	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		// Strip a leading markdown heading marker if present.
		trimmed = strings.TrimLeft(trimmed, "#")
		trimmed = strings.TrimSpace(trimmed)
		if trimmed == "" {
			continue
		}
		return trimmed
	}

	return ""
}

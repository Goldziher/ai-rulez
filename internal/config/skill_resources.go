package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/logger"
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
// or assets/) trigger a warning naming the item and the offending directory.
// The Agent Skills spec defines only these three as canonical resource kinds.
//
// itemKind is one of the ItemKind constants and only affects diagnostics.
func LoadResources(root, itemKind string) ([]SkillResource, error) {
	var resources []SkillResource

	for _, kind := range skillResourceKinds {
		kindDir := filepath.Join(root, kind)
		// Lstat (not Stat) so a symlinked kind directory — e.g. an
		// installed skill with `references -> /etc` — is reported as a
		// symlink and refused. Stat would follow the link and let
		// WalkDir into an attacker-controlled tree, bypassing the
		// per-entry symlink guard below.
		info, err := os.Lstat(kindDir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, oops.With("path", kindDir).Wrapf(err, "stat %s resource dir", itemKind)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			logger.Warn("Skipping symlinked resource directory", "owner", itemKind, "kind", kind, "path", kindDir)
			continue
		}
		if !info.IsDir() {
			continue
		}

		kindResources, err := walkSkillResourceDir(root, kindDir, kind)
		if err != nil {
			return nil, err
		}
		resources = append(resources, kindResources...)
	}

	// Warn about unrecognized subdirectories so authors know their content is
	// not being included in generated output. Only directories are checked;
	// regular files in the item root (like SKILL.md, .gitignore) are expected
	// and not warned about.
	warnings, err := unrecognizedSubdirectoryWarnings(root, itemKind)
	if err != nil {
		return nil, err
	}
	for _, warning := range warnings {
		// The kind doubles as the attribute key, so the line reads
		// skill=<path> or command=<path>.
		logger.Warn(
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
func unrecognizedSubdirectoryWarnings(root, itemKind string) ([]resourceWarning, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, oops.With("path", root).Wrapf(err, "read %s directory for unrecognized subdirs", itemKind)
	}

	recognized := make(map[string]bool, len(skillResourceKinds))
	for _, kind := range skillResourceKinds {
		recognized[kind] = true
	}

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
			logger.Warn("Could not stat subdirectory", "owner", itemKind, "path", root, "name", entry.Name(), "error", err)
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
func walkSkillResourceDir(skillDir, kindDir, kind string) ([]SkillResource, error) {
	var resources []SkillResource

	err := filepath.WalkDir(kindDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return oops.With("path", path).Wrapf(walkErr, "walk skill resource dir")
		}
		if d.IsDir() {
			return nil
		}

		// Skip symlinks. WalkDir surfaces symlinks via d.Type() but reads
		// the target's bytes when we call os.ReadFile, which would let an
		// installed skill exfiltrate arbitrary files (e.g. references/key
		// → /etc/passwd) through the rendered SKILL.md output. Skill
		// resources must be regular files inside the skill directory.
		if d.Type()&os.ModeSymlink != 0 {
			logger.Warn("Skipping symlink in skill resources", "path", path)
			return nil
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

		// nolint:gosec // G122: WalkDir callback uses path; the symlink
		// check above prevents following arbitrary links into /etc/passwd.
		// A residual TOCTOU window exists between Lstat and ReadFile, but
		// the threat model — single-user CLI run with the user's own
		// privileges over user-controlled skill content — does not include
		// concurrent attacker swaps. Switching to os.Root would close the
		// window but at the cost of cross-platform portability of file
		// mode handling.
		data, err := os.ReadFile(path)
		if err != nil {
			return oops.With("path", path).Wrapf(err, "read skill resource")
		}

		// Capture file mode so the executable bit on bundled scripts survives
		// the round-trip through generation.
		info, err := d.Info()
		if err != nil {
			return oops.With("path", path).Wrapf(err, "stat skill resource")
		}

		resource := SkillResource{
			Kind:    kind,
			RelPath: relToSkill,
			Content: data,
			Mode:    info.Mode().Perm(),
		}

		if kind == SkillKindReferences && strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
			resource.Description = extractResourceDescription(data)
		}

		resources = append(resources, resource)
		return nil
	})
	if err != nil {
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

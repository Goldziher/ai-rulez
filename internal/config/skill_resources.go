package config

import (
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

// skillResourceKinds is the ordered list of subdirectories the loader walks
// under a skill root. Order is meaningful: it determines the order
// resources appear in the rendered SKILL.md index.
var skillResourceKinds = []string{
	SkillKindReferences,
	SkillKindScripts,
	SkillKindAssets,
}

// LoadSkillResources walks a skill directory and returns its supporting files
// from references/, scripts/, and assets/ subdirectories. Files are read as
// raw bytes so binary assets round-trip without UTF-8 corruption.
//
// For markdown references the function also extracts a short description,
// preferring the frontmatter `description` field, falling back to the first
// non-empty content line (with markdown heading markers stripped).
//
// Missing subdirectories are not an error — they simply contribute no
// resources. Walk failures within a subdirectory are returned to the caller
// so the loader can decide whether to surface or warn.
func LoadSkillResources(skillDir string) ([]SkillResource, error) {
	var resources []SkillResource

	for _, kind := range skillResourceKinds {
		kindDir := filepath.Join(skillDir, kind)
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
			return nil, oops.With("path", kindDir).Wrapf(err, "stat skill resource dir")
		}
		if info.Mode()&os.ModeSymlink != 0 {
			logger.Warn("Skipping symlinked skill resource directory", "kind", kind, "path", kindDir)
			continue
		}
		if !info.IsDir() {
			continue
		}

		kindResources, err := walkSkillResourceDir(skillDir, kindDir, kind)
		if err != nil {
			return nil, err
		}
		resources = append(resources, kindResources...)
	}

	return resources, nil
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

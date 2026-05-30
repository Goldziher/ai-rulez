package presets

import (
	"path"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
)

// RenderSkillResourcesIndex renders a "Resources" section that lists the
// skill's bundled supporting files so the agent can discover them and decide
// which to read on demand. This implements progressive disclosure: SKILL.md
// stays small, references are loaded only when relevant.
//
// The output is empty when the skill has no resources. Otherwise the section
// is grouped by kind (References, Scripts, Assets) with each entry rendered
// as a relative markdown link so the format works in every preset that
// outputs SKILL.md to a directory.
//
// Takes ContentFile by pointer because the struct is wide (80 bytes with
// the Resources slice header); a value copy on every preset render path
// would be measurable on monorepos with many skills.
func RenderSkillResourcesIndex(skill *config.ContentFile) string {
	if len(skill.Resources) == 0 {
		return ""
	}

	groups := groupResourcesByKind(skill.Resources)
	if len(groups[config.SkillKindReferences])+
		len(groups[config.SkillKindScripts])+
		len(groups[config.SkillKindAssets]) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("\n\n## Resources\n\n")
	b.WriteString("This skill bundles supporting files. Read them on demand when the task calls for them — don't bulk-load.\n")

	if refs := groups[config.SkillKindReferences]; len(refs) > 0 {
		b.WriteString("\n### References\n\n")
		for _, r := range refs {
			b.WriteString("- [`")
			b.WriteString(r.RelPath)
			b.WriteString("`](")
			b.WriteString(r.RelPath)
			b.WriteString(")")
			if r.Description != "" {
				b.WriteString(" — ")
				b.WriteString(r.Description)
			}
			b.WriteString("\n")
		}
	}

	if scripts := groups[config.SkillKindScripts]; len(scripts) > 0 {
		b.WriteString("\n### Scripts\n\n")
		for _, r := range scripts {
			b.WriteString("- `")
			b.WriteString(r.RelPath)
			b.WriteString("`\n")
		}
	}

	if assets := groups[config.SkillKindAssets]; len(assets) > 0 {
		b.WriteString("\n### Assets\n\n")
		for _, r := range assets {
			b.WriteString("- `")
			b.WriteString(r.RelPath)
			b.WriteString("`\n")
		}
	}

	return b.String()
}

// InlineSkillResources renders all reference resources inline as labeled
// sections. Used by presets like continue.dev that emit a single concatenated
// rules file with no place to read separate reference files from.
//
// Scripts and assets are not inlined — they're not text the agent should
// "read" — and they're silently dropped for these single-file presets, since
// the runtime has no working directory to invoke or load them from.
func InlineSkillResources(skill *config.ContentFile) string {
	if len(skill.Resources) == 0 {
		return ""
	}

	groups := groupResourcesByKind(skill.Resources)
	refs := groups[config.SkillKindReferences]
	if len(refs) == 0 {
		return ""
	}

	var b strings.Builder
	for _, r := range refs {
		// Strip frontmatter from inlined references — readers shouldn't see it.
		_, body := config.ParseFrontmatterPublic(string(r.Content))
		body = strings.TrimSpace(body)
		if body == "" {
			continue
		}
		b.WriteString("\n## Reference: ")
		b.WriteString(referenceDisplayName(r.RelPath))
		b.WriteString("\n\n")
		b.WriteString(body)
		b.WriteString("\n")
	}
	return b.String()
}

// SkillResourceOutputs returns one OutputFile per resource, rooted at
// skillDir. Files are emitted with RawContent so the writer skips header
// injection and trailing-newline normalization — preserves scripts and
// binary assets verbatim.
//
// Each kind subdirectory (references/, scripts/, assets/) is also emitted
// as an explicit IsDir entry. This is what gets the directory into the
// generator's managedDirs set so cleanManagedDirs sweeps stale files inside
// it on subsequent generations — without these markers, deleted reference
// files would leak between regenerations.
func SkillResourceOutputs(skill *config.ContentFile, skillDir string) []config.OutputFile {
	if len(skill.Resources) == 0 {
		return nil
	}
	outputs := make([]config.OutputFile, 0, len(skill.Resources)+3)

	// Emit each kind directory that has at least one resource, plus parent
	// directories of any nested resources. Use a set so we never duplicate.
	dirSet := make(map[string]bool)
	for _, r := range skill.Resources {
		curr := filepath.FromSlash(r.RelPath)
		// Defensive guard: an absolute RelPath would make filepath.Dir
		// converge on "/" (or volume root) and never reach ".", spinning
		// the loop forever. LoadSkillResources only produces relative
		// paths, but skip rather than hang if a caller violates that.
		if path.IsAbs(r.RelPath) || filepath.IsAbs(curr) {
			continue
		}
		for {
			curr = filepath.Dir(curr)
			if curr == "." {
				break
			}
			dirSet[curr] = true
		}
	}
	for d := range dirSet {
		outputs = append(outputs, config.OutputFile{
			Path:  filepath.Join(skillDir, d),
			IsDir: true,
		})
	}

	for _, r := range skill.Resources {
		if path.IsAbs(r.RelPath) || filepath.IsAbs(filepath.FromSlash(r.RelPath)) {
			continue
		}
		// r.RelPath uses forward slashes. filepath.Join handles platform
		// separators correctly via filepath.FromSlash.
		outputs = append(outputs, config.OutputFile{
			Path:       filepath.Join(skillDir, filepath.FromSlash(r.RelPath)),
			RawContent: append([]byte(nil), r.Content...),
			Mode:       r.Mode,
		})
	}
	return outputs
}

func groupResourcesByKind(resources []config.SkillResource) map[string][]config.SkillResource {
	groups := make(map[string][]config.SkillResource, 3)
	for _, r := range resources {
		groups[r.Kind] = append(groups[r.Kind], r)
	}
	return groups
}

// referenceDisplayName extracts a human-friendly name from a reference path,
// e.g. "references/api.md" -> "api". For nested paths under references/,
// preserves the subpath so disambiguation is possible.
func referenceDisplayName(relPath string) string {
	name := strings.TrimPrefix(relPath, config.SkillKindReferences+"/")
	name = strings.TrimSuffix(name, ".md")
	return name
}

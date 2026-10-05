package generator

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/usage"
	"github.com/zeebo/blake3"
)

// skillOutputBase is the file name every harness reads a skill from.
const skillOutputBase = "SKILL.md"

// skillsIndexOutput builds the skills index when [usage] skills_index is on. It
// returns false when the feature is off, so default output is unchanged.
//
// The index is derived from the profile-resolved content tree and the per-preset
// outputs, never from files on disk, so it is identical whether or not the tree
// has been generated yet and is byte-stable across runs.
func (g *Generator) skillsIndexOutput(content *config.ContentTree, allOutputs map[string][]config.OutputFile) (config.OutputFile, bool) {
	if g.config.Usage == nil || !g.config.Usage.SkillsIndex || content == nil {
		return config.OutputFile{}, false
	}

	outputsByID := skillOutputsByID(allOutputs, g.config.BaseDir)
	index := usage.Index{SchemaVersion: usage.IndexSchemaVersion}
	add := func(domain string, skills, commands []config.ContentFile) {
		for i := range skills {
			if record, ok := g.skillRecord(domain, usage.KindSkill, config.SkillID(skills[i]), &skills[i], outputsByID); ok {
				index.Skills = append(index.Skills, record)
			}
		}
		for i := range commands {
			id := commandSkillID(commands[i].Name)
			if record, ok := g.skillRecord(domain, usage.KindCommand, id, &commands[i], outputsByID); ok && len(record.Outputs) > 0 {
				index.Skills = append(index.Skills, record)
			}
		}
	}
	add("", content.Skills, content.Commands)
	names := make([]string, 0, len(content.Domains))
	for name := range content.Domains {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if domain := content.Domains[name]; domain != nil {
			add(name, domain.Skills, domain.Commands)
		}
	}

	data, err := index.Marshal()
	if err != nil {
		return config.OutputFile{}, false
	}
	return config.OutputFile{
		Path:       filepath.Join(g.config.ConfigDir, usage.IndexFileName),
		RawContent: data,
	}, true
}

// skillRecord builds the index record of one skill.
func (g *Generator) skillRecord(domain, kind, id string, skill *config.ContentFile, outputsByID map[string]map[string][]string) (usage.SkillRecord, bool) {
	if id == "" {
		return usage.SkillRecord{}, false
	}
	record := usage.SkillRecord{
		ID:      id,
		Kind:    kind,
		Domain:  domain,
		Source:  g.relToProject(skill.Path),
		Hash:    skillHash(skill),
		Outputs: map[string][]string{},
	}
	if skill.Metadata != nil {
		record.Owner = strings.TrimSpace(skill.Metadata.Extra["owner"])
		record.Version = strings.TrimSpace(skill.Metadata.Extra["version"])
	}
	for preset, paths := range outputsByID[id] {
		record.Outputs[preset] = paths
	}
	return record, true
}

// commandSkillID is the directory name a command is written under when a
// harness runs it as a skill: the lowercased name with spaces and underscores
// as dashes.
func commandSkillID(name string) string {
	return strings.NewReplacer(" ", "-", "_", "-").Replace(strings.ToLower(name))
}

// relToProject renders a source path relative to the project root with forward
// slashes. Builtin content keeps its builtin:// identity.
func (g *Generator) relToProject(path string) string {
	if path == "" || strings.Contains(path, "://") {
		return path
	}
	if rel, err := filepath.Rel(g.config.BaseDir, path); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(path)
}

// skillHash digests the authored skill: SKILL.md as written on disk (the whole
// file, frontmatter included) followed by every bundled resource in path order.
// Content carried only in memory (builtin skills) is hashed as loaded.
func skillHash(skill *config.ContentFile) string {
	hasher := blake3.New()
	data, err := os.ReadFile(skill.Path)
	if err != nil {
		data = []byte(skill.Content)
	}
	writeDigest(hasher, "SKILL.md", data)

	resources := append([]config.SkillResource(nil), skill.Resources...)
	sort.Slice(resources, func(a, b int) bool { return resources[a].RelPath < resources[b].RelPath })
	for _, resource := range resources {
		writeDigest(hasher, resource.RelPath, resource.Content)
	}
	return fmt.Sprintf("blake3:%x", hasher.Sum(nil))
}

// writeDigest feeds one named, length-prefixed blob to the hasher, so two
// different splits of the same bytes cannot collide.
func writeDigest(hasher *blake3.Hasher, name string, data []byte) {
	//nolint:errcheck // blake3 writes never fail
	_, _ = hasher.WriteString(name + "\x00" + strconv.Itoa(len(data)) + "\x00")
	_, _ = hasher.Write(data) //nolint:errcheck // blake3 writes never fail
}

// skillOutputsByID maps skill id to preset to the SKILL.md paths that preset
// writes, relative to the project root. Paths shared by several presets
// (.agents/skills) are listed under each.
func skillOutputsByID(allOutputs map[string][]config.OutputFile, baseDir string) map[string]map[string][]string {
	result := map[string]map[string][]string{}
	for preset, outputs := range allOutputs {
		for _, output := range outputs {
			if output.IsDir || filepath.Base(output.Path) != skillOutputBase {
				continue
			}
			if config.InferOutputKind(output.Path, baseDir) != config.OutputKindSkill {
				continue
			}
			id := filepath.Base(filepath.Dir(output.Path))
			rel, err := filepath.Rel(baseDir, output.Path)
			if err != nil {
				continue
			}
			if result[id] == nil {
				result[id] = map[string][]string{}
			}
			name := strings.Trim(preset, "<>")
			result[id][name] = append(result[id][name], filepath.ToSlash(rel))
		}
	}
	for _, byPreset := range result {
		for preset := range byPreset {
			sort.Strings(byPreset[preset])
		}
	}
	return result
}

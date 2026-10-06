package generator

import (
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/samber/oops"
)

const skillEntryFile = "SKILL.md"

// ServedSkillFile is one file of a served skill, exactly as `generate` writes it.
type ServedSkillFile struct {
	// RelPath is the path relative to the skill directory, "/"-separated.
	RelPath string
	Content []byte
}

// ServedSkill is one skill of the effective set for a profile and preset, with
// the bytes `generate` would write for it and where it came from.
type ServedSkill struct {
	// ID is the skill directory name in the generated output.
	ID string
	// Domain is the domain that owns the skill; empty for root content.
	Domain string
	// Source is the authored path (relative to the config directory when it is
	// inside it), or the installed-skill source repository.
	Source string
	// Ref is the git ref an installed skill is pinned to; empty otherwise.
	Ref string
	// Pinned reports that Ref is a full commit SHA rather than a moving ref.
	Pinned   bool
	Keywords []string
	// Triggers are the phrases the skill declares (frontmatter `triggers`) that
	// should make an agent look for it.
	Triggers []string
	Category string
	// Delivery is the skill's effective delivery (static, served or both).
	Delivery config.Delivery
	// Commit is the commit a remote skill source was resolved to; empty otherwise.
	Commit string
	// Trust is the security-scan level the server applies to this skill
	// ("error" or "warn"); empty means the server's default for the origin.
	Trust string
	// Verbatim marks a skill whose files are served exactly as found (a skill
	// source): none of them is a generated file, so no generated-header
	// normalization applies to its lock digest.
	Verbatim bool
	// Files lists SKILL.md first, then the supporting files in path order.
	Files []ServedSkillFile
}

// ServedSkills renders the effective skill set of a profile with one preset and
// returns it in memory, without writing or deleting anything. The bytes equal
// the generated SKILL.md and resource files for the same profile and preset, so
// a consumer serving them serves what the generated tree would contain.
//
// preset selects the rendering (its frontmatter dialect and placement rules).
// An empty preset picks the first configured preset that produces skills. The
// resolved preset name is returned alongside the skills.
func (g *Generator) ServedSkills(profile, preset string) (resolved string, skills []ServedSkill, err error) {
	candidates := g.skillPresetCandidates(preset)
	if len(candidates) == 0 {
		return "", nil, oops.Errorf("no preset configured that can render skills")
	}
	for _, name := range candidates {
		skills, err = g.servedSkillsForPreset(profile, name)
		if err != nil {
			return "", nil, err
		}
		if len(skills) > 0 || preset != "" {
			return name, skills, nil
		}
	}
	return candidates[len(candidates)-1], nil, nil
}

func (g *Generator) skillPresetCandidates(preset string) []string {
	if preset != "" {
		return []string{preset}
	}
	var names []string
	for i := range g.config.Presets {
		p := g.config.Presets[i]
		names = append(names, p.GetName())
	}
	return names
}

func (g *Generator) servedSkillsForPreset(profile, preset string) ([]ServedSkill, error) {
	cfg := *g.config
	cfg.ServeMode = true
	cfg.Presets = []config.Preset{{BuiltIn: preset, Name: preset}}
	for i := range g.config.Presets {
		if g.config.Presets[i].GetName() == preset {
			cfg.Presets = []config.Preset{g.config.Presets[i]}
			break
		}
	}
	sub := NewGenerator(&cfg)
	sub.role = g.role // g.config already carries the role's delivery and overrides

	generateMu.Lock()
	defer generateMu.Unlock()
	sub.beginRun()
	outputs, activeProfile, err := sub.collectOutputs(profile)
	if err != nil {
		return nil, oops.Wrapf(err, "render preset %q", preset)
	}
	tree, err := sub.getContentForProfile(activeProfile)
	if err != nil {
		return nil, err
	}
	owners := skillOwners(&cfg, tree)

	var skills []ServedSkill
	for _, out := range outputs {
		if out.IsDir || filepath.Base(out.Path) != skillEntryFile {
			continue
		}
		dir := filepath.Dir(out.Path)
		if filepath.Base(filepath.Dir(dir)) != "skills" {
			continue
		}
		skill := ServedSkill{ID: filepath.Base(dir)}
		if owner, ok := owners[skill.ID]; ok {
			skill.Domain, skill.Source, skill.Ref, skill.Pinned = owner.domain, owner.source, owner.ref, owner.pinned
			skill.Keywords, skill.Category = owner.keywords, owner.category
			skill.Triggers, skill.Delivery = owner.triggers, owner.delivery
		}
		skill.Files = append([]ServedSkillFile{{RelPath: skillEntryFile, Content: []byte(sub.finalContent(out))}},
			skillResourceFiles(outputs, dir)...)
		skills = append(skills, skill)
	}
	sort.Slice(skills, func(i, j int) bool { return skills[i].ID < skills[j].ID })
	return skills, nil
}

// skillResourceFiles collects the raw supporting files written under a skill
// directory, in path order. SKILL.md itself is not one of them.
func skillResourceFiles(outputs []config.OutputFile, dir string) []ServedSkillFile {
	var files []ServedSkillFile
	for _, res := range outputs {
		if res.IsDir || res.RawContent == nil {
			continue
		}
		rel, err := filepath.Rel(dir, res.Path)
		if err != nil || strings.HasPrefix(rel, "..") || rel == skillEntryFile {
			continue
		}
		files = append(files, ServedSkillFile{RelPath: filepath.ToSlash(rel), Content: res.RawContent})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].RelPath < files[j].RelPath })
	return files
}

type skillOwner struct {
	domain, source, ref string
	pinned              bool
	keywords            []string
	triggers            []string
	delivery            config.Delivery
	category            string
}

// skillOwners maps a skill ID to the domain and origin that supplied it. Root
// content wins over domains, mirroring generation order; domains are visited in
// name order so the result is deterministic.
func skillOwners(cfg *config.Config, tree *config.ContentTree) map[string]skillOwner {
	installed := map[string]config.InstalledSkillConfig{}
	for i := range cfg.InstalledSkills {
		installed[cfg.InstalledSkills[i].Name] = cfg.InstalledSkills[i]
	}
	owners := map[string]skillOwner{}
	add := func(domain string, files []config.ContentFile) {
		for i := range files {
			f := files[i]
			id := config.SkillID(f)
			if _, dup := owners[id]; dup {
				continue
			}
			o := skillOwner{domain: domain, source: relSource(cfg.BaseDir, f.Path), delivery: cfg.EffectiveDelivery(f, domain, nil)}
			if f.Metadata != nil {
				o.keywords, o.category = f.Metadata.Keywords, f.Metadata.Category
				o.triggers = f.Metadata.ExtraList("triggers")
			}
			if inst, ok := installed[f.Name]; ok {
				o.source, o.ref, o.pinned = inst.Source, inst.RequestedRef(), isCommitSHA(inst.Ref)
			}
			owners[id] = o
		}
	}
	add("", tree.Skills)
	names := make([]string, 0, len(tree.Domains))
	for name := range tree.Domains {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		add(name, tree.Domains[name].Skills)
	}
	return owners
}

func relSource(baseDir, p string) string {
	if p == "" || baseDir == "" || !filepath.IsAbs(p) {
		return filepath.ToSlash(p)
	}
	if rel, err := filepath.Rel(baseDir, p); err == nil && !strings.HasPrefix(rel, "..") {
		return path.Clean(filepath.ToSlash(rel))
	}
	return filepath.ToSlash(p)
}

func isCommitSHA(ref string) bool {
	if len(ref) != 40 {
		return false
	}
	for _, r := range ref {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

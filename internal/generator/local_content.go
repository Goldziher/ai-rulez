package generator

import (
	"sort"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/presets"
	"github.com/Goldziher/ai-rulez/internal/logger"
)

const kindSkill = "skill"

// perItemContent returns a copy of a content tree that keeps only the content
// written as one file per item: skills, agents, commands and checks, at the root
// and in every domain. Rules and context are dropped: they have their own local outputs
// (personal rule files and the .local root).
func perItemContent(t *config.ContentTree) *config.ContentTree {
	out := &config.ContentTree{
		Skills:   t.Skills,
		Agents:   t.Agents,
		Commands: t.Commands,
		Checks:   t.Checks,
		Domains:  make(map[string]*config.Domain, len(t.Domains)),
	}
	for name, d := range t.Domains {
		if d == nil || len(d.Skills)+len(d.Agents)+len(d.Commands)+len(d.Checks) == 0 {
			continue
		}
		out.Domains[name] = &config.Domain{
			Name: d.Name, Skills: d.Skills, Agents: d.Agents, Commands: d.Commands, Checks: d.Checks,
			Builtin: d.Builtin, BuiltinScoped: d.BuiltinScoped, FromInclude: d.FromInclude,
		}
	}
	return out
}

func hasPerItemContent(t *config.ContentTree) bool {
	if len(t.Skills)+len(t.Agents)+len(t.Commands)+len(t.Checks) > 0 {
		return true
	}
	for _, d := range t.Domains {
		if d != nil && len(d.Skills)+len(d.Agents)+len(d.Commands)+len(d.Checks) > 0 {
			return true
		}
	}
	return false
}

// withLocalItems returns the shared tree extended with the local per-item
// content. A local domain that shares its name with a shared one contributes its
// items to that domain.
func withLocalItems(shared, local *config.ContentTree) *config.ContentTree {
	out := &config.ContentTree{
		Rules:    shared.Rules,
		Context:  shared.Context,
		Skills:   append(append([]config.ContentFile(nil), shared.Skills...), local.Skills...),
		Agents:   append(append([]config.ContentFile(nil), shared.Agents...), local.Agents...),
		Commands: append(append([]config.ContentFile(nil), shared.Commands...), local.Commands...),
		Checks:   append(append([]config.ContentFile(nil), shared.Checks...), local.Checks...),
		Domains:  make(map[string]*config.Domain, len(shared.Domains)+len(local.Domains)),
	}
	for name, d := range shared.Domains {
		out.Domains[name] = d
	}
	for name, ld := range local.Domains {
		sd, ok := out.Domains[name]
		if !ok {
			out.Domains[name] = ld
			continue
		}
		merged := *sd
		merged.Skills = append(append([]config.ContentFile(nil), sd.Skills...), ld.Skills...)
		merged.Agents = append(append([]config.ContentFile(nil), sd.Agents...), ld.Agents...)
		merged.Commands = append(append([]config.ContentFile(nil), sd.Commands...), ld.Commands...)
		merged.Checks = append(append([]config.ContentFile(nil), sd.Checks...), ld.Checks...)
		out.Domains[name] = &merged
	}
	return out
}

// checkLocalCollisions fails when a local skill, command, agent or check has the
// same ID as a shared one: both would be written to the same file, and a machine-local
// file must never replace a shared one. Skills and commands share one output
// namespace, so they are compared together.
func checkLocalCollisions(shared, local *config.ContentTree) error {
	type source struct{ kind, name, path string }
	index := func(t *config.ContentTree) map[string]source {
		byKey := map[string]source{}
		for _, f := range presets.AllSkills(t) {
			byKey[kindSkill+":"+f.Name] = source{kindSkill, f.Name, f.Path}
		}
		for _, f := range presets.AllCommands(t) {
			byKey[kindSkill+":"+f.Name] = source{"command", f.Name, f.Path}
		}
		for _, f := range presets.AllAgents(t) {
			byKey["agent:"+f.Name] = source{"agent", f.Name, f.Path}
		}
		for _, f := range presets.AllChecks(t) {
			byKey["check:"+strings.ToLower(f.Name)] = source{"check", f.Name, f.Path}
		}
		return byKey
	}
	sharedItems, localItems := index(shared), index(local)

	keys := make([]string, 0, len(localItems))
	for k := range localItems {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if s, clash := sharedItems[k]; clash {
			l := localItems[k]
			return oops.
				With("name", l.name, "shared", s.path, "local", l.path).
				Hint("Rename the local item or remove one of them: a local file may not replace a shared one").
				Errorf("local %s %q (%s) collides with shared %s %q (%s)", l.kind, l.name, l.path, s.kind, s.name, s.path)
		}
	}
	return nil
}

// appendLocalItemOutputs renders the machine-local skills, agents and commands as
// per-item files only, at the paths the shared ones use. The presets render the
// shared content extended with the local items; every file whose path the shared
// render did not already produce is a local item's file. Root files and
// aggregates are never taken from this render, so the committed ones stay
// untouched. Items keep their `targets`, since the presets filter by them.
func (g *Generator) appendLocalItemOutputs(allOutputs map[string][]config.OutputFile, cfg *config.Config,
	local *config.ContentTree,
) error {
	if !hasPerItemContent(local) {
		return nil
	}
	items := perItemContent(local)
	if err := checkLocalCollisions(cfg.Content, items); err != nil {
		return err
	}

	all, err := renderLocalPresets(cfg, withLocalItems(cfg.Content, items))
	if err != nil {
		return oops.Wrapf(err, "render local skills, agents and commands")
	}

	known := make(map[string]bool)
	for _, outs := range allOutputs {
		for _, o := range outs {
			known[o.Path] = true
		}
	}
	builtin := make(map[string]bool)
	for _, p := range g.config.Presets {
		if p.IsBuiltIn() {
			builtin[p.GetName()] = true
		}
	}
	g.warnDroppedItems(cfg, items, known, builtin)
	appendNewLocalOutputs(allOutputs, all, known, builtin)
	shared := map[string][]config.OutputFile{sharedOutputsKey: sharedLocalSkills(cfg, items)}
	appendNewLocalOutputs(allOutputs, shared, known, map[string]bool{sharedOutputsKey: true})
	return nil
}

// appendNewLocalOutputs appends, per preset in name order, the files of rendered
// whose path is not known yet, as LocalOnly outputs; directories are left out.
func appendNewLocalOutputs(allOutputs, rendered map[string][]config.OutputFile, known, only map[string]bool) {
	names := make([]string, 0, len(rendered))
	for name := range rendered {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if !only[name] {
			continue
		}
		for _, o := range rendered[name] {
			if o.IsDir || known[o.Path] {
				continue
			}
			known[o.Path] = true
			o.LocalOnly = true
			allOutputs[name] = append(allOutputs[name], o)
		}
	}
}

// renderLocalPresets renders the presets for a content tree that carries local
// items. With agents_md on, a consumer's own root AGENTS.md and own skills
// directory are dropped, as in the shared render: otherwise every shared skill
// would reappear as a "local" file in the consumer's own directory.
func renderLocalPresets(cfg *config.Config, content *config.ContentTree) (map[string][]config.OutputFile, error) {
	rendered := *cfg
	rendered.Content = content
	rendered.Analysis = nil
	all, err := config.GeneratePresets(&rendered)
	if err != nil {
		return nil, err //nolint:wrapcheck // wrapped by the caller
	}
	if !cfg.AgentsMD {
		return all, nil
	}
	targeted := skillTargets(presets.SkillTargets(content))
	for name, outs := range all {
		if consumer, ok := config.SharedOutputConsumerFor(name); ok {
			all[name] = dropOwnOutputs(outs, cfg.BaseDir, name, consumer, targeted)
		}
	}
	return all, nil
}

// readsSharedSkills reports whether agents_md is on and a configured built-in
// preset reads the shared .agents/skills tree.
func readsSharedSkills(cfg *config.Config) bool {
	if !cfg.AgentsMD {
		return false
	}
	for _, preset := range cfg.Presets {
		if !preset.IsBuiltIn() {
			continue
		}
		if consumer, ok := config.SharedOutputConsumerFor(preset.BuiltIn); ok && consumer.Reads(config.SharedAgentSkills) {
			return true
		}
	}
	return false
}

// sharedLocalSkills renders the local skills into the shared .agents/skills tree
// (untargeted ones only, like the shared skills), so they land next to the shared
// skills the consumers read. Directories are left out.
func sharedLocalSkills(cfg *config.Config, items *config.ContentTree) []config.OutputFile {
	if !readsSharedSkills(cfg) {
		return nil
	}
	var files []config.OutputFile
	for _, o := range presets.SharedAgentSkills(items, cfg.BaseDir) {
		if !o.IsDir {
			files = append(files, o)
		}
	}
	return files
}

// warnDroppedItems warns, per preset and kind, about local skills, agents,
// commands and checks the preset aggregates into a shared file (or has no output for): they
// get no per-item file, and a machine-local item is never merged into a file the
// team shares, so they are not written at all.
func (g *Generator) warnDroppedItems(cfg *config.Config, items *config.ContentTree, known, builtin map[string]bool) {
	for name, dropped := range g.droppedItems(cfg, items, known, builtin) {
		logger.Warn("Machine-local skills, agents, commands or checks have no per-item output for this preset and were not written",
			"preset", name, "items", strings.Join(dropped, ", "))
	}
}

// droppedItems maps each builtin preset to the labels of the local items it
// produces no file for. Each kind is rendered on its own so one kind's files
// cannot hide another's absence.
func (g *Generator) droppedItems(cfg *config.Config, items *config.ContentTree, known, builtin map[string]bool,
) map[string][]string {
	kinds := []struct {
		label  string
		files  func(*config.ContentTree) []config.ContentFile
		narrow func(*config.ContentTree) *config.ContentTree
	}{
		{kindSkill, presets.AllSkills, func(t *config.ContentTree) *config.ContentTree { return onlyKind(t, true, false, false, false) }},
		{"agent", presets.AllAgents, func(t *config.ContentTree) *config.ContentTree { return onlyKind(t, false, true, false, false) }},
		{"command", presets.AllCommands, func(t *config.ContentTree) *config.ContentTree { return onlyKind(t, false, false, true, false) }},
		{"check", presets.AllChecks, func(t *config.ContentTree) *config.ContentTree { return onlyKind(t, false, false, false, true) }},
	}
	dropped := map[string][]string{}
	for _, kind := range kinds {
		files := kind.files(items)
		if len(files) == 0 {
			continue
		}
		narrowed := kind.narrow(items)
		all, err := renderLocalPresets(cfg, withLocalItems(cfg.Content, narrowed))
		if err != nil {
			continue // the combined render reports the error
		}
		sharedPlaced := kind.label == kindSkill && anyUnknown(sharedLocalSkills(cfg, narrowed), known)
		for name, outs := range all {
			if !builtin[name] {
				continue
			}
			consumer, isConsumer := config.SharedOutputConsumerFor(name)
			if (sharedPlaced && isConsumer && consumer.Reads(config.SharedAgentSkills)) || anyUnknown(outs, known) {
				continue
			}
			for _, f := range files {
				dropped[name] = append(dropped[name], kind.label+" "+f.Name)
			}
		}
	}
	for name := range dropped {
		sort.Strings(dropped[name])
	}
	return dropped
}

// anyUnknown reports whether outputs hold a file whose path is not known yet.
func anyUnknown(outputs []config.OutputFile, known map[string]bool) bool {
	for _, o := range outputs {
		if !o.IsDir && !known[o.Path] {
			return true
		}
	}
	return false
}

// onlyKind keeps the selected per-item kinds of a content tree.
func onlyKind(t *config.ContentTree, skills, agents, commands, checks bool) *config.ContentTree {
	pick := func(on bool, files []config.ContentFile) []config.ContentFile {
		if on {
			return files
		}
		return nil
	}
	out := &config.ContentTree{
		Skills:   pick(skills, t.Skills),
		Agents:   pick(agents, t.Agents),
		Commands: pick(commands, t.Commands),
		Checks:   pick(checks, t.Checks),
		Domains:  make(map[string]*config.Domain, len(t.Domains)),
	}
	for name, d := range t.Domains {
		out.Domains[name] = &config.Domain{
			Name: d.Name, Skills: pick(skills, d.Skills), Agents: pick(agents, d.Agents), Commands: pick(commands, d.Commands),
			Checks:  pick(checks, d.Checks),
			Builtin: d.Builtin, BuiltinScoped: d.BuiltinScoped, FromInclude: d.FromInclude,
		}
	}
	return out
}

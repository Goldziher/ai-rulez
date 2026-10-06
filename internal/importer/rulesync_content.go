package importer

import (
	"bytes"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"gopkg.in/yaml.v3"
)

const curatedDir = ".curated"

const curatedReason = "installed from a remote source by `rulesync install`; imported as a local file and no longer tracked"

// fmField is one frontmatter key to write, in order.
type fmField struct {
	key string
	val any
}

// renderDoc writes optional frontmatter followed by the body.
func renderDoc(fields []fmField, body string) ([]byte, error) {
	body = strings.TrimLeft(body, "\n")
	if len(fields) == 0 {
		return ensureNewline(body), nil
	}
	node := &yaml.Node{Kind: yaml.MappingNode}
	for _, f := range fields {
		val := &yaml.Node{}
		if err := val.Encode(f.val); err != nil {
			return nil, err
		}
		node.Content = append(node.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: f.key}, val)
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(node); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return ensureNewline("---\n" + buf.String() + "---\n\n" + body), nil
}

// listOf reads a YAML string or list as a list of strings; a string is split on commas.
func listOf(v any) []string {
	var out []string
	add := func(s string) {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	switch t := v.(type) {
	case string:
		for _, s := range strings.Split(t, ",") {
			add(s)
		}
	case []any:
		for _, e := range t {
			add(fmt.Sprint(e))
		}
	}
	return out
}

func stringOf(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

func sectionOf(meta map[string]any, name string) map[string]any {
	m, _ := meta[name].(map[string]any)
	return m
}

// frontmatterOf parses the frontmatter of a rulesync file, reporting a lenient read.
func (b *rulesyncPlanner) frontmatterOf(file, text string) (meta map[string]any, body string) {
	fm, body, has := splitFrontmatter(text)
	if !has {
		return map[string]any{}, body
	}
	meta, lenient := parseFrontmatter(fm)
	if lenient {
		b.p.add(newFinding(StatusApproximated, file, "frontmatter", "",
			"frontmatter is not valid YAML as written; it was read leniently, review the result"))
	}
	return meta, body
}

// reportLeftovers reports every frontmatter key (and section key) that was not
// carried: nothing rulesync wrote is dropped without a finding. taken lists
// "key" or "section.key" entries the importer used.
func (b *rulesyncPlanner) reportLeftovers(file string, meta map[string]any, taken map[string]bool) {
	keys := make([]string, 0, len(meta))
	for k := range meta {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if taken[k] {
			continue
		}
		sec, isSection := meta[k].(map[string]any)
		if !isSection {
			b.p.add(newFinding(StatusDropped, file, k, "", "frontmatter key has no ai-rulez equivalent"))
			continue
		}
		var left []string
		for sk := range sec {
			if !taken[k+"."+sk] {
				left = append(left, sk)
			}
		}
		if len(left) == 0 {
			continue
		}
		sort.Strings(left)
		b.p.add(newFinding(StatusDropped, file, k, "", "tool-specific section is not carried (keys: "+
			strings.Join(left, ", ")+"); ai-rulez has no per-tool overrides"))
	}
}

// nameOf derives the item name from a path below a content directory; a leading
// .curated segment is dropped and reported.
func (b *rulesyncPlanner) nameOf(file, rel string) (name string, synthesized bool) {
	rest := rel
	if strings.HasPrefix(rel, curatedDir+"/") {
		rest = strings.TrimPrefix(rel, curatedDir+"/")
		b.p.add(newFinding(StatusApproximated, file, "", "", curatedReason))
	}
	return itemName(rest)
}

func (b *rulesyncPlanner) readContent(file string) (string, bool) {
	data, ok := nativeImporter{}.readOrReport(b.p, b.r, file)
	if !ok {
		return "", false
	}
	return normalizeText(string(data)), true
}

func catchAllGlob(g string) bool { return g == "**/*" || g == "**" }

// splitCatchAll separates the globs that scope a rule from the "match everything"
// patterns rulesync writes for a rule that applies to every file.
func splitCatchAll(globs []string) (specific []string, everything bool) {
	for _, g := range globs {
		if catchAllGlob(g) {
			everything = true
			continue
		}
		specific = append(specific, g)
	}
	if everything {
		return nil, true
	}
	return specific, false
}

func (b *rulesyncPlanner) importRules(root string) {
	dir := path.Join(root, "rules")
	if _, ok := b.r.exists(dir); !ok {
		return
	}
	for _, rel := range b.r.walkFiles(dir, b.onSkip) {
		file := path.Join(dir, rel)
		if !isMarkdown(rel) {
			b.p.add(newFinding(StatusUnsupported, file, "", "", "not a Markdown rule file"))
			continue
		}
		text, ok := b.readContent(file)
		if !ok {
			continue
		}
		b.importRule(file, rel, text)
	}
}

func (b *rulesyncPlanner) importRule(file, rel, text string) {
	p := b.p
	meta, body := b.frontmatterOf(file, text)
	if strings.TrimSpace(body) == "" {
		p.add(newFinding(StatusDropped, file, "", "", "rule body is empty"))
		return
	}
	localRoot, _ := meta["localRoot"].(bool)
	name, synth := b.nameOf(file, rel)
	nameFinding(p, file, KindRule, name, synth)
	taken := map[string]bool{"root": true, "localRoot": true, "targets": true, "description": true, "globs": true}

	desc := stringOf(meta["description"])
	globs := globsFrom(meta["globs"])
	specific, everything := splitCatchAll(globs)
	fm := map[string]any{}
	note := func(section, key, target, reason string) {
		taken[section+"."+key] = true
		p.add(newFinding(StatusApproximated, file, section+"."+key, target, reason))
	}

	if desc == "" {
		for _, sec := range []string{"cursor", "devin", "antigravity", "kiro"} {
			if d := stringOf(sectionOf(meta, sec)["description"]); d != "" {
				desc = d
				note(sec, "description", "description", "used as the description because the rule has none of its own")
				break
			}
		}
	}
	if !everything && len(specific) == 0 {
		specific = b.activationHint(file, meta, fm, note)
	}
	if desc != "" {
		fm["description"] = desc
	}
	if len(specific) > 0 {
		list := make([]any, len(specific))
		for i, g := range specific {
			list[i] = g
		}
		fm["globs"] = list
	}
	if everything {
		fm["alwaysApply"] = true
	}
	rule, findings := translateRule(file, fm, false)
	p.Findings = append(p.Findings, findings...)
	rule.Targets = mapItemTargets(p, file, listOf(meta["targets"]))
	// A Cursor flag that agrees with the result carries nothing more to report.
	cursor := sectionOf(meta, "cursor")
	if on, isBool := cursor["alwaysApply"].(bool); isBool && on == (rule.Activation == "") {
		taken["cursor.alwaysApply"] = true
	}
	if g, has := cursor["globs"]; has && len(globsFrom(g)) == 0 {
		taken["cursor.globs"] = true
	}
	b.reportLeftovers(file, meta, taken)

	kind := KindRule
	if root, _ := meta["root"].(bool); root && !localRoot {
		kind = KindContext
		p.add(newFinding(StatusApproximated, file, "root", "context/"+name+".md",
			"root rules are written into every tool's root file; ai-rulez does that with context"))
	}
	if localRoot {
		// A localRoot rule is rulesync's personal root file (CLAUDE.local.md): the
		// machine-local context tree is its counterpart, and git ignores it.
		kind = KindContext // root is implied: a root rule that is also local is the local one
		p.add(newFinding(StatusApproximated, file, "localRoot", localDir+"/context/"+name+".md",
			"personal rule imported into the machine-local tree .ai-rulez/local/, which git ignores; it never reaches a committed output"))
	}
	out, err := renderRule(rule, body)
	if err != nil {
		p.add(newFinding(StatusDropped, file, "", "", "frontmatter could not be written: "+err.Error()))
		return
	}
	b.addItem(Item{Kind: kind, Name: name, Sources: []string{file}, Main: out, Local: localRoot})
}

// activationHint reads the activation a rule states through a tool section when
// its canonical globs say nothing: Cursor's alwaysApply, Devin and Antigravity's
// trigger, Kiro's inclusion. It fills fm for translateRule, and returns the globs
// it found (the section's own, or Claude Code's paths and Cursor's globs).
func (b *rulesyncPlanner) activationHint(file string, meta, fm map[string]any, note func(section, key, target, reason string)) []string {
	if cursor := sectionOf(meta, "cursor"); cursor != nil {
		if on, _ := cursor["alwaysApply"].(bool); on {
			fm["alwaysApply"] = true
			note("cursor", "alwaysApply", "rules", "always apply; the rule applies to every tool")
			return nil
		}
	}
	for _, sec := range []string{"devin", "antigravity"} {
		trigger := stringOf(sectionOf(meta, sec)["trigger"])
		if trigger == "" {
			continue
		}
		fm["trigger"] = trigger
		note(sec, "trigger", "rules", "activation taken from this section; it applies to every tool")
		if trigger == "glob" {
			if g := globsFrom(sectionOf(meta, sec)["globs"]); len(g) > 0 {
				note(sec, "globs", "rules", "globs taken from this section")
				return g
			}
		}
		return nil
	}
	if kiro := sectionOf(meta, "kiro"); stringOf(kiro["inclusion"]) != "" {
		fm["inclusion"] = stringOf(kiro["inclusion"])
		note("kiro", "inclusion", "rules", "activation taken from this section; it applies to every tool")
		if g := globsFrom(kiro["fileMatchPattern"]); len(g) > 0 {
			note("kiro", "fileMatchPattern", "rules", "globs taken from this section")
			return g
		}
		return nil
	}
	if g := globsFrom(sectionOf(meta, "claudecode")["paths"]); len(g) > 0 {
		note("claudecode", "paths", "rules", "globs taken from this section because the rule has none of its own")
		return g
	}
	if g := globsFrom(sectionOf(meta, "cursor")["globs"]); len(g) > 0 {
		note("cursor", "globs", "rules", "globs taken from this section because the rule has none of its own")
		return g
	}
	return nil
}

// importFlatKind imports commands, subagents or checks: Markdown files with
// frontmatter, below <root>/<sub>.
func (b *rulesyncPlanner) importFlatKind(root, sub string, kind Kind) {
	dir := path.Join(root, sub)
	if _, ok := b.r.exists(dir); !ok {
		return
	}
	for _, rel := range b.r.walkFiles(dir, b.onSkip) {
		file := path.Join(dir, rel)
		if !isMarkdown(rel) {
			b.p.add(newFinding(StatusUnsupported, file, "", "", "only Markdown "+string(kind)+" files are imported"))
			continue
		}
		text, ok := b.readContent(file)
		if !ok {
			continue
		}
		meta, body := b.frontmatterOf(file, text)
		if strings.TrimSpace(body) == "" {
			b.p.add(newFinding(StatusDropped, file, "", "", string(kind)+" body is empty"))
			continue
		}
		name, synth := b.nameOf(file, rel)
		nameFinding(b.p, file, kind, name, synth)
		if strings.Contains(strings.TrimPrefix(rel, curatedDir+"/"), "/") {
			b.p.add(newFinding(StatusApproximated, file, "name", string(kind)+"s/"+name,
				"the directory is part of the name; ai-rulez "+string(kind)+"s are flat"))
		}
		var fields []fmField
		switch kind {
		case KindCommand:
			fields = b.commandFields(file, meta)
		case KindAgent:
			fields = b.agentFields(file, name, meta)
		case KindCheck:
			fields = b.checkFields(file, meta)
		case KindRule, KindContext, KindSkill:
		}
		out, err := renderDoc(fields, body)
		if err != nil {
			b.p.add(newFinding(StatusDropped, file, "", "", "frontmatter could not be written: "+err.Error()))
			continue
		}
		b.addItem(Item{Kind: kind, Name: name, Sources: []string{file}, Main: out})
	}
}

func targetsField(p *Plan, file string, meta map[string]any) []fmField {
	if t := mapItemTargets(p, file, listOf(meta["targets"])); len(t) > 0 {
		return []fmField{{"targets", t}}
	}
	return nil
}

func (b *rulesyncPlanner) commandFields(file string, meta map[string]any) []fmField {
	var fields []fmField
	if d := stringOf(meta["description"]); d != "" {
		fields = append(fields, fmField{"description", d})
	}
	fields = append(fields, targetsField(b.p, file, meta)...)
	b.reportLeftovers(file, meta, map[string]bool{"description": true, "targets": true})
	return fields
}

func (b *rulesyncPlanner) agentFields(file, name string, meta map[string]any) []fmField {
	taken := map[string]bool{"name": true, "description": true, "targets": true}
	agentName := stringOf(meta["name"])
	if agentName == "" {
		agentName = name
	}
	fields := []fmField{{"name", agentName}}
	if d := stringOf(meta["description"]); d != "" {
		fields = append(fields, fmField{"description", d})
	}
	claude := sectionOf(meta, "claudecode")
	lift := func(key string, val any) {
		taken["claudecode."+key] = true
		fields = append(fields, fmField{key, val})
		b.p.add(newFinding(StatusApproximated, file, "claudecode."+key, key,
			"lifted out of the claudecode section; it applies to every preset"))
	}
	if m := stringOf(claude["model"]); m != "" {
		if m == "inherit" {
			taken["claudecode.model"] = true // the default; nothing to carry
		} else {
			lift("model", m)
		}
	}
	if t := listOf(claude["tools"]); len(t) > 0 {
		lift("tools", t)
	}
	if s := listOf(claude["skills"]); len(s) > 0 {
		lift("skills", s)
	}
	if e := stringOf(claude["effort"]); e != "" {
		lift("effort", e)
	}
	fields = append(fields, targetsField(b.p, file, meta)...)
	b.reportLeftovers(file, meta, taken)
	return fields
}

func (b *rulesyncPlanner) checkFields(file string, meta map[string]any) []fmField {
	var fields []fmField
	if d := stringOf(meta["description"]); d != "" {
		fields = append(fields, fmField{"description", d})
	}
	taken := map[string]bool{"description": true, "targets": true, "severity": true, "tools": true}
	if s := strings.ToLower(stringOf(meta["severity"])); s != "" {
		valid := false
		for _, v := range config.CheckSeverities {
			valid = valid || v == s
		}
		if valid {
			fields = append(fields, fmField{"severity", s})
		} else {
			b.p.add(newFinding(StatusDropped, file, "severity", "", "unknown severity "+s+"; use low, medium, high or critical"))
		}
	}
	if t := listOf(meta["tools"]); len(t) > 0 {
		fields = append(fields, fmField{"tools", t})
	}
	fields = append(fields, targetsField(b.p, file, meta)...)
	b.reportLeftovers(file, meta, taken)
	return fields
}

func (b *rulesyncPlanner) importSkills(root string) {
	dir := path.Join(root, "skills")
	if _, ok := b.r.exists(dir); !ok {
		return
	}
	for _, e := range b.r.dirEntries(dir, b.onSkip) {
		if !e.IsDir() {
			b.p.add(newFinding(StatusDropped, path.Join(dir, e.Name()), "", "", "expected a skill directory"))
			continue
		}
		if e.Name() == curatedDir {
			for _, c := range b.r.dirEntries(path.Join(dir, curatedDir), b.onSkip) {
				if c.IsDir() {
					b.p.add(newFinding(StatusApproximated, path.Join(dir, curatedDir, c.Name()), "", "", curatedReason))
					b.importSkill(path.Join(dir, curatedDir, c.Name()), c.Name())
				}
			}
			continue
		}
		b.importSkill(path.Join(dir, e.Name()), e.Name())
	}
}

func (b *rulesyncPlanner) importSkill(skillDir, dirName string) {
	p := b.p
	skillFile := path.Join(skillDir, "SKILL.md")
	if _, ok := b.r.exists(skillFile); !ok {
		p.add(newFinding(StatusDropped, skillDir, "", "", "no SKILL.md in the directory"))
		return
	}
	text, ok := b.readContent(skillFile)
	if !ok {
		return
	}
	name, _ := safeName(dirName)
	if name != dirName {
		p.add(newFinding(StatusApproximated, skillDir, "name", "skills/"+name, "skill directory renamed to a valid name"))
	}
	meta, body := b.frontmatterOf(skillFile, text)
	taken := map[string]bool{"name": true, "description": true, "targets": true}
	if n := stringOf(meta["name"]); n != "" && n != name {
		p.add(newFinding(StatusApproximated, skillFile, "name", "skills/"+name,
			"name "+n+" rewritten to match the skill directory"))
	}
	fields := []fmField{{"name", name}}
	if d := stringOf(meta["description"]); d != "" {
		fields = append(fields, fmField{"description", d})
	}
	for _, k := range []string{"license", "compatibility", "metadata"} {
		if v, ok := meta[k]; ok {
			taken[k] = true
			fields = append(fields, fmField{k, v})
		}
	}
	for _, k := range []string{"disable-model-invocation", "user-invocable"} {
		if v, ok := meta[k]; ok {
			taken[k] = true
			fields = append(fields, fmField{k, v})
			p.add(newFinding(StatusApproximated, skillFile, k, "",
				"kept as written; ai-rulez has no field for it, so only presets that copy unknown keys render it"))
		}
	}
	if tools := sectionOf(meta, "claudecode")["allowed-tools"]; tools != nil {
		taken["claudecode.allowed-tools"] = true
		fields = append(fields, fmField{"allowed-tools", tools})
		p.add(newFinding(StatusApproximated, skillFile, "claudecode.allowed-tools", "allowed-tools",
			"lifted out of the claudecode section; it applies to every preset"))
	}
	fields = append(fields, targetsField(p, skillFile, meta)...)
	b.reportLeftovers(skillFile, meta, taken)
	out, err := renderDoc(fields, body)
	if err != nil {
		p.add(newFinding(StatusDropped, skillFile, "", "", "frontmatter could not be written: "+err.Error()))
		return
	}
	it := Item{Kind: KindSkill, Name: name, Sources: []string{skillDir}, Main: out}
	for _, rel := range b.r.walkFiles(skillDir, b.onSkip) {
		if rel == "SKILL.md" {
			continue
		}
		file := path.Join(skillDir, rel)
		res, err := b.r.read(file)
		if err != nil {
			p.add(newFinding(StatusDropped, file, "", "", skipReasonOr(err)))
			continue
		}
		if b.r.isGeneratedFile(file, res) {
			p.add(newFinding(StatusDropped, file, "", "", "generated by ai-rulez; not imported as source"))
			continue
		}
		it.Resources = append(it.Resources, File{Path: rel, Data: res, Exec: b.r.executable(file)})
	}
	b.addItem(it)
}

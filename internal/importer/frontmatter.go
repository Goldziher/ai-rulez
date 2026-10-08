package importer

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/frontmatter"
	"gopkg.in/yaml.v3"
)

// splitFrontmatter separates a leading YAML block from the body. Line endings
// are normalised to LF and a BOM is removed, like the lock does.
func splitFrontmatter(content string) (fm, body string, has bool) {
	content = strings.TrimPrefix(content, "\ufeff")
	content = strings.ReplaceAll(strings.ReplaceAll(content, "\r\n", "\n"), "\r", "\n")
	block := frontmatter.SplitString(content)
	if !block.Closed {
		return "", content, false
	}
	return strings.TrimSuffix(block.Raw, "\n"), block.Body, true
}

// parseFrontmatter reads a frontmatter block into a map. Editors write values
// YAML rejects (an unquoted `globs: **/*.ts` starts with an alias), so on a YAML
// error it quotes such values and parses again, which keeps block scalars and
// lists intact; only if that fails too does it use a line reader. lenient is
// true when the block was not valid YAML as written.
func parseFrontmatter(fm string) (out map[string]any, lenient bool) {
	out = map[string]any{}
	if err := yaml.Unmarshal([]byte(fm), &out); err == nil && out != nil {
		return out, false
	}
	out = map[string]any{}
	if err := yaml.Unmarshal([]byte(quoteIndicatorValues(fm)), &out); err == nil && out != nil {
		return out, true
	}
	return lineFrontmatter(fm), true
}

// yamlIndicator lists the characters a plain YAML scalar cannot start with that
// editors nevertheless leave unquoted in globs.
const yamlIndicator = "*&!%@`"

// quoteIndicatorValues single-quotes the values of `key: value` and `- value`
// lines that start with a YAML indicator character.
func quoteIndicatorValues(fm string) string {
	lines := strings.Split(fm, "\n")
	for i, line := range lines {
		trimmed := strings.TrimLeft(line, " \t")
		indent := line[:len(line)-len(trimmed)]
		prefix, value := "", ""
		switch {
		case strings.HasPrefix(trimmed, "- "):
			prefix, value = "- ", strings.TrimSpace(trimmed[2:])
		default:
			k, v, ok := strings.Cut(trimmed, ":")
			if !ok || strings.ContainsAny(k, " \t\"'") {
				continue
			}
			prefix, value = k+": ", strings.TrimSpace(v)
		}
		if value == "" || !strings.ContainsRune(yamlIndicator, rune(value[0])) {
			continue
		}
		lines[i] = indent + prefix + "'" + strings.ReplaceAll(value, "'", "''") + "'"
	}
	return strings.Join(lines, "\n")
}

// lineFrontmatter is the last-resort reader: top-level key: value pairs, `- `
// lists and indented block scalars.
func lineFrontmatter(fm string) map[string]any {
	out := map[string]any{}
	lines := strings.Split(fm, "\n")
	var key string
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "" || strings.HasPrefix(trimmed, "#"):
		case strings.HasPrefix(trimmed, "- ") && key != "":
			list := as[[]any](out[key])
			out[key] = append(list, unquote(strings.TrimPrefix(trimmed, "- ")))
		default:
			k, v, ok := strings.Cut(trimmed, ":")
			if !ok {
				continue
			}
			key = strings.TrimSpace(k)
			v = strings.TrimSpace(v)
			if v == "" {
				delete(out, key)
				continue
			}
			if v[0] == '|' || v[0] == '>' {
				var block []string
				for i+1 < len(lines) && (strings.TrimSpace(lines[i+1]) == "" || strings.HasPrefix(lines[i+1], " ") || strings.HasPrefix(lines[i+1], "\t")) {
					i++
					block = append(block, strings.TrimSpace(lines[i]))
				}
				sep := "\n"
				if v[0] == '>' {
					sep = " "
				}
				text := strings.Join(block, sep)
				text = strings.TrimRight(text, "\n ")
				out[key] = text
				continue
			}
			out[key] = unquote(v)
		}
	}
	return out
}

func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && (s[0] == '"' && s[len(s)-1] == '"' || s[0] == '\'' && s[len(s)-1] == '\'') {
		return s[1 : len(s)-1]
	}
	return s
}

// splitGlobs splits a comma-separated glob string, keeping commas inside braces.
func splitGlobs(s string) []string {
	var out []string
	depth, start := 0, 0
	add := func(end int) {
		if g := unquote(strings.TrimSpace(s[start:end])); g != "" {
			out = append(out, g)
		}
	}
	for i, c := range s {
		switch c {
		case '{':
			depth++
		case '}':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				add(i)
				start = i + 1
			}
		}
	}
	add(len(s))
	return out
}

func globsFrom(v any) []string {
	switch t := v.(type) {
	case string:
		return splitGlobs(t)
	case []any:
		var out []string
		for _, e := range t {
			if s, ok := e.(string); ok {
				out = append(out, splitGlobs(s)...)
			}
		}
		return out
	}
	return nil
}

// ruleFM is the ai-rulez rule frontmatter the importer writes, in a fixed order.
type ruleFM struct {
	Description string   `yaml:"description,omitempty"`
	Activation  string   `yaml:"activation,omitempty"`
	Globs       []string `yaml:"globs,omitempty"`
	Priority    string   `yaml:"priority,omitempty"`
	Targets     []string `yaml:"targets,omitempty"`
}

var validActivations = map[string]bool{litAlways: true, litGlob: true, autoFrom: true, ActionManual: true}

// translateRule maps a native rule file's frontmatter onto ai-rulez rule
// frontmatter. Keys come from Cursor (alwaysApply, globs, description), VS Code
// Copilot (applyTo), Kiro (inclusion, fileMatchPattern), Devin and Windsurf
// (trigger), Claude Code and Cline (paths) and ai-rulez itself. cursor marks the
// Cursor dialect, where a file without frontmatter is manual. Anything else is
// dropped with a finding; nothing is kept silently.
func translateRule(source string, fm map[string]any, cursor bool) (ruleFM, []Finding) {
	t := &ruleTranslator{source: source}
	keys := make([]string, 0, len(fm))
	for k := range fm {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		t.translateKey(k, fm[k])
	}
	t.out.Globs = dedupeSorted(t.out.Globs)
	t.settleActivation(cursor)
	return t.out, t.findings
}

// ruleTranslator accumulates the translation of one rule's frontmatter.
type ruleTranslator struct {
	source     string
	out        ruleFM
	findings   []Finding
	activation string
}

var (
	triggerActivations   = map[string]string{"always_on": litAlways, litGlob: litGlob, "model_decision": autoFrom, ActionManual: ActionManual}
	inclusionActivations = map[string]string{litAlways: litAlways, "fileMatch": litGlob, autoFrom: autoFrom, ActionManual: ActionManual}
)

func (t *ruleTranslator) approximate(field, reason string) {
	t.findings = append(t.findings, newFinding(StatusApproximated, t.source, field, rulesDir, reason))
}

func (t *ruleTranslator) drop(key, reason string) {
	t.findings = append(t.findings, newFinding(StatusDropped, t.source, key, "", reason))
}

func (t *ruleTranslator) translateKey(k string, v any) {
	switch k {
	case litDescription:
		if s, ok := v.(string); ok {
			t.out.Description = strings.TrimSpace(s)
		}
	case litGlobs, "paths", "fileMatchPattern":
		t.out.Globs = append(t.out.Globs, globsFrom(v)...)
	case "applyTo":
		t.applyTo(v)
	case "alwaysApply":
		if b, ok := v.(bool); ok && b {
			t.activation = litAlways
		}
	case "trigger":
		t.mapActivation(triggerActivations, k, v, "trigger")
	case "inclusion":
		t.mapActivation(inclusionActivations, k, v, "inclusion")
	case "activation":
		t.setActivation(k, v)
	case "priority":
		t.setPriority(k, v)
	case litName:
		// The rule is named after its file.
	default:
		t.drop(k, "frontmatter key has no ai-rulez equivalent")
	}
}

// applyTo reads a Copilot applyTo: a catch-all glob means always on.
func (t *ruleTranslator) applyTo(v any) {
	g := globsFrom(v)
	if len(g) == 1 && (g[0] == "**" || g[0] == "**/*") {
		t.activation = litAlways
		return
	}
	t.out.Globs = append(t.out.Globs, g...)
}

func (t *ruleTranslator) setActivation(key string, v any) {
	if s := fmt.Sprint(v); validActivations[s] {
		t.activation = s
	} else {
		t.drop(key, "unknown activation value")
	}
}

func (t *ruleTranslator) setPriority(key string, v any) {
	if s := fmt.Sprint(v); config.Priority(s).IsValid() {
		t.out.Priority = s
	} else {
		t.drop(key, "unknown priority value")
	}
}

// mapActivation sets the activation from a tool-specific value through table.
func (t *ruleTranslator) mapActivation(table map[string]string, key string, v any, what string) {
	if a, ok := table[fmt.Sprint(v)]; ok {
		t.activation = a
		return
	}
	t.drop(key, "unknown "+what+" value")
}

// settleActivation picks the activation the keys left open and normalizes it.
func (t *ruleTranslator) settleActivation(cursor bool) {
	switch {
	case t.activation != "":
	case len(t.out.Globs) > 0:
		t.activation = litGlob
	case cursor && t.out.Description != "":
		t.activation = autoFrom
		t.approximate(litDescription, "description-only auto-attach is approximated as activation: auto")
	case cursor:
		t.activation = ActionManual
		t.approximate("frontmatter", "a Cursor rule with no alwaysApply, globs or description is applied manually")
	}
	if t.activation == litGlob && len(t.out.Globs) == 0 {
		t.activation = ""
		t.approximate("activation", "glob activation without globs; the rule is always applied")
	}
	if t.activation == litAlways {
		if len(t.out.Globs) > 0 {
			// An always-on rule applies everywhere; keeping its globs would scope it.
			t.approximate(litGlobs, "the rule is always on, so its globs are dropped (ai-rulez would otherwise scope it to them)")
			t.out.Globs = nil
		}
		// always is ai-rulez's default; keep it explicit only when it carries meaning.
		t.activation = ""
	}
	t.out.Activation = t.activation
}

func dedupeSorted(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// renderRule writes a rule file: optional frontmatter, then the body.
func renderRule(meta ruleFM, body string) ([]byte, error) {
	var b strings.Builder
	if meta.Description != "" || meta.Activation != "" || len(meta.Globs) > 0 || meta.Priority != "" || len(meta.Targets) > 0 {
		var buf bytes.Buffer
		enc := yaml.NewEncoder(&buf)
		enc.SetIndent(2)
		if err := enc.Encode(meta); err != nil {
			return nil, err
		}
		if err := enc.Close(); err != nil {
			return nil, err
		}
		b.WriteString("---\n")
		b.Write(buf.Bytes())
		b.WriteString("---\n\n")
	}
	b.WriteString(strings.TrimLeft(body, "\n"))
	s := b.String()
	if !strings.HasSuffix(s, "\n") {
		s += "\n"
	}
	return []byte(s), nil
}

// auxKnownKeys are the frontmatter keys of agents, commands and skills that
// ai-rulez understands (its own fields, the shared agent model and the Agent
// Skills spec). Every other key is tool specific.
var auxKnownKeys = map[string]bool{
	litName: true, litDescription: true, "model": true, litTools: true, skillsDir: true, litTargets: true,
	"aliases": true, "keywords": true, "priority": true, "usage": true, "shortcut": true, "category": true,
	"effort": true, "short-description": true, litLicense: true, "compatibility": true, litMetadata: true,
	"allowed-tools": true, "activation": true, litGlobs: true, "paths": true,
}

// reviewAuxFrontmatter inspects the frontmatter of an agent, command or skill.
// Tool-specific keys stay in the file, so nothing is lost, but each one is
// reported: ai-rulez has no field for it, so only a preset that copies unknown
// keys will render it. A comma-separated `tools` string (Claude Code) becomes a
// YAML list. It returns the possibly rewritten text.
func reviewAuxFrontmatter(p *Plan, source, text string) string {
	fm, body, has := splitFrontmatter(text)
	if !has {
		return text
	}
	meta, lenient := parseFrontmatter(fm)
	if lenient {
		p.add(newFinding(StatusApproximated, source, "frontmatter", "",
			"frontmatter is not valid YAML as written; it was read leniently, review the result"))
	}
	keys := make([]string, 0, len(meta))
	for k := range meta {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if !auxKnownKeys[k] {
			p.add(newFinding(StatusApproximated, source, k, "",
				"tool-specific key kept verbatim; ai-rulez has no field for it, so only presets that copy unknown keys render it"))
		}
	}
	if tools, ok := meta[litTools].(string); ok && strings.Contains(tools, ",") && !lenient {
		var list []string
		for _, t := range strings.Split(tools, ",") {
			if t = strings.TrimSpace(t); t != "" {
				list = append(list, t)
			}
		}
		if rewritten, ok := replaceFrontmatterKey(fm, litTools, "tools:\n  - "+strings.Join(list, "\n  - ")); ok {
			p.add(newFinding(StatusApproximated, source, litTools, litTools,
				"comma-separated tools string converted to a list"))
			return "---\n" + rewritten + "\n---\n" + body
		}
	}
	return text
}

// replaceFrontmatterKey replaces the top-level `key: value` line of a
// frontmatter block with repl.
func replaceFrontmatterKey(fm, key, repl string) (string, bool) {
	lines := strings.Split(fm, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, key+":") {
			lines[i] = repl
			return strings.Join(lines, "\n"), true
		}
	}
	return fm, false
}

// setFrontmatterName rewrites the `name:` of a SKILL.md to match its directory,
// which ai-rulez names the skill after. A file without a name is left alone.
func setFrontmatterName(text, name string) string {
	fm, body, has := splitFrontmatter(text)
	if !has {
		return text
	}
	meta, _ := parseFrontmatter(fm)
	if cur, ok := meta[litName]; !ok || fmt.Sprint(cur) == name {
		return text
	}
	rewritten, ok := replaceFrontmatterKey(fm, litName, "name: "+name)
	if !ok {
		return text
	}
	return "---\n" + rewritten + "\n---\n" + body
}

// nestUnknownKeys moves the top-level frontmatter keys that known does not list
// under `metadata:` (merged into an existing metadata map), so a root file such
// as a CLAUDE.md that carries title, applies_to or updated keys becomes a
// context file the strict validator accepts without losing the values. moved
// names the keys it moved. Frontmatter that is not a plain YAML mapping is
// returned unchanged.
func nestUnknownKeys(text string, known map[string]bool) (out string, moved []string) {
	fm, body, has := splitFrontmatter(text)
	if !has {
		return text, nil
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(fm), &doc); err != nil || doc.Kind != yaml.DocumentNode ||
		len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return text, nil
	}
	root := doc.Content[0]
	var keep, unknown []*yaml.Node
	var metadata *yaml.Node
	for i := 0; i+1 < len(root.Content); i += 2 {
		k, v := root.Content[i], root.Content[i+1]
		switch {
		case k.Value == litMetadata && v.Kind == yaml.MappingNode:
			metadata = v
			keep = append(keep, k, v)
		case known[k.Value]:
			keep = append(keep, k, v)
		default:
			unknown = append(unknown, k, v)
			moved = append(moved, k.Value)
		}
	}
	if len(unknown) == 0 {
		return text, nil
	}
	if metadata != nil {
		metadata.Content = append(metadata.Content, unknown...)
		root.Content = keep
	} else {
		meta := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: unknown}
		keep = append(keep, &yaml.Node{Kind: yaml.ScalarNode, Tag: litYAMLStr, Value: litMetadata}, meta)
		root.Content = keep
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return text, nil
	}
	if err := enc.Close(); err != nil {
		return text, nil
	}
	return "---\n" + strings.TrimRight(buf.String(), "\n") + "\n---\n" + body, moved
}

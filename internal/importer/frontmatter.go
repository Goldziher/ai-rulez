package importer

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"gopkg.in/yaml.v3"
)

// splitFrontmatter separates a leading YAML block from the body. Line endings
// are normalised to LF and a BOM is removed, like the lock does.
func splitFrontmatter(content string) (fm, body string, has bool) {
	content = strings.TrimPrefix(content, "\ufeff")
	content = strings.ReplaceAll(strings.ReplaceAll(content, "\r\n", "\n"), "\r", "\n")
	if !strings.HasPrefix(content, "---\n") {
		return "", content, false
	}
	rest := content[4:]
	if strings.HasPrefix(rest, "---\n") || rest == "---" {
		return "", strings.TrimPrefix(strings.TrimPrefix(rest, "---"), "\n"), true
	}
	idx := strings.Index(rest, "\n---")
	for idx >= 0 {
		after := rest[idx+4:]
		if after == "" || strings.HasPrefix(after, "\n") {
			return rest[:idx], strings.TrimPrefix(after, "\n"), true
		}
		next := strings.Index(rest[idx+1:], "\n---")
		if next < 0 {
			break
		}
		idx += 1 + next
	}
	return "", content, false
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

var validActivations = map[string]bool{"always": true, "glob": true, "auto": true, "manual": true}

// translateRule maps a native rule file's frontmatter onto ai-rulez rule
// frontmatter. Keys come from Cursor (alwaysApply, globs, description), VS Code
// Copilot (applyTo), Kiro (inclusion, fileMatchPattern), Devin and Windsurf
// (trigger), Claude Code and Cline (paths) and ai-rulez itself. cursor marks the
// Cursor dialect, where a file without frontmatter is manual. Anything else is
// dropped with a finding; nothing is kept silently.
func translateRule(source string, fm map[string]any, cursor bool) (ruleFM, []Finding) {
	var out ruleFM
	var findings []Finding
	keys := make([]string, 0, len(fm))
	for k := range fm {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	activation := ""
	approximate := func(field, reason string) {
		findings = append(findings, newFinding(StatusApproximated, source, field, "rules", reason))
	}
	for _, k := range keys {
		v := fm[k]
		switch k {
		case "description":
			if s, ok := v.(string); ok {
				out.Description = strings.TrimSpace(s)
			}
		case "globs", "paths", "fileMatchPattern":
			out.Globs = append(out.Globs, globsFrom(v)...)
		case "applyTo":
			g := globsFrom(v)
			if len(g) == 1 && (g[0] == "**" || g[0] == "**/*") {
				activation = "always"
				continue
			}
			out.Globs = append(out.Globs, g...)
		case "alwaysApply":
			if b, ok := v.(bool); ok && b {
				activation = "always"
			}
		case "trigger":
			switch fmt.Sprint(v) {
			case "always_on":
				activation = "always"
			case "glob":
				activation = "glob"
			case "model_decision":
				activation = "auto"
			case "manual":
				activation = "manual"
			default:
				findings = append(findings, newFinding(StatusDropped, source, k, "", "unknown trigger value"))
			}
		case "inclusion":
			switch fmt.Sprint(v) {
			case "always":
				activation = "always"
			case "fileMatch":
				activation = "glob"
			case "auto":
				activation = "auto"
			case "manual":
				activation = "manual"
			default:
				findings = append(findings, newFinding(StatusDropped, source, k, "", "unknown inclusion value"))
			}
		case "activation":
			if s := fmt.Sprint(v); validActivations[s] {
				activation = s
			} else {
				findings = append(findings, newFinding(StatusDropped, source, k, "", "unknown activation value"))
			}
		case "priority":
			if s := fmt.Sprint(v); config.Priority(s).IsValid() {
				out.Priority = s
			} else {
				findings = append(findings, newFinding(StatusDropped, source, k, "", "unknown priority value"))
			}
		case "name":
			// The rule is named after its file.
		default:
			findings = append(findings, newFinding(StatusDropped, source, k, "",
				"frontmatter key has no ai-rulez equivalent"))
		}
	}
	out.Globs = dedupeSorted(out.Globs)

	switch {
	case activation != "":
	case len(out.Globs) > 0:
		activation = "glob"
	case cursor && out.Description != "":
		activation = "auto"
		approximate("description", "description-only auto-attach is approximated as activation: auto")
	case cursor:
		activation = "manual"
		approximate("frontmatter", "a Cursor rule with no alwaysApply, globs or description is applied manually")
	}
	if activation == "glob" && len(out.Globs) == 0 {
		activation = ""
		approximate("activation", "glob activation without globs; the rule is always applied")
	}
	if activation == "always" {
		if len(out.Globs) > 0 {
			// An always-on rule applies everywhere; keeping its globs would scope it.
			findings = append(findings, newFinding(StatusApproximated, source, "globs", "rules",
				"the rule is always on, so its globs are dropped (ai-rulez would otherwise scope it to them)"))
			out.Globs = nil
		}
		// always is ai-rulez's default; keep it explicit only when it carries meaning.
		activation = ""
	}
	out.Activation = activation
	return out, findings
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
	"name": true, "description": true, "model": true, "tools": true, "skills": true, "targets": true,
	"aliases": true, "keywords": true, "priority": true, "usage": true, "shortcut": true, "category": true,
	"effort": true, "short-description": true, "license": true, "compatibility": true, "metadata": true,
	"allowed-tools": true, "activation": true, "globs": true, "paths": true,
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
	if tools, ok := meta["tools"].(string); ok && strings.Contains(tools, ",") && !lenient {
		var list []string
		for _, t := range strings.Split(tools, ",") {
			if t = strings.TrimSpace(t); t != "" {
				list = append(list, t)
			}
		}
		if rewritten, ok := replaceFrontmatterKey(fm, "tools", "tools:\n  - "+strings.Join(list, "\n  - ")); ok {
			p.add(newFinding(StatusApproximated, source, "tools", "tools",
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
	if cur, ok := meta["name"]; !ok || fmt.Sprint(cur) == name {
		return text
	}
	rewritten, ok := replaceFrontmatterKey(fm, "name", "name: "+name)
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
		case k.Value == "metadata" && v.Kind == yaml.MappingNode:
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
		root.Content = append(keep, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "metadata"}, meta)
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

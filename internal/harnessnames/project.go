package harnessnames

import (
	"fmt"
	"strings"
)

// ParseProject decodes a project addition (.ai-rulez/harness-names/<preset>.toml). It
// has the layout of an embedded file; sources, quotes and dates are optional because
// the project, not a vendor page, is the evidence. A group needs no source and its
// collision outcome defaults to unknown.
func ParseProject(data []byte, preset string) (*Table, error) {
	t, err := Parse(data)
	if err != nil {
		return nil, err
	}
	if t.Schema != SchemaVersion {
		return nil, fmt.Errorf("schema = %d, want %d", t.Schema, SchemaVersion)
	}
	if t.Preset != preset {
		return nil, fmt.Errorf("preset = %q, want %q (the file name)", t.Preset, preset)
	}
	if t.Severity != "" && t.Severity != SeverityError && t.Severity != SeverityWarning {
		return nil, fmt.Errorf("severity = %q, want %q or %q", t.Severity, SeverityError, SeverityWarning)
	}
	for i := range t.Group {
		g := &t.Group[i]
		if g.UserCollision == "" {
			g.UserCollision = Unknown
		}
		if g.Match == "" {
			g.Match = MatchExact
		}
		if err := validateGroup(g, t, fmt.Sprintf("group %d", i+1), false); err != nil {
			return nil, err
		}
	}
	t.reindex()
	return t, nil
}

// WithProject returns a copy of t with the groups of p added. The added groups are
// reported at the severity of p (warning unless p says error). t may be nil, for a
// preset the embedded tables do not know.
func (t *Table) WithProject(p *Table) *Table {
	if p == nil {
		return t
	}
	out := &Table{Schema: SchemaVersion, Preset: p.Preset, Completeness: Partial, Severity: SeverityWarning,
		InvocationSpaces: [][]string{{KindCommand, KindSkill}, {KindAgent}}, declared: map[int]string{}}
	if t != nil {
		cp := *t
		out = &cp
		out.Source = append([]Source(nil), t.Source...)
		out.Group = append([]Group(nil), t.Group...)
		out.declared = map[int]string{}
		for k, v := range t.declared {
			out.declared[k] = v
		}
	}
	out.Project = true
	sev := p.Severity
	if sev == "" {
		sev = SeverityWarning
	}
	remap := map[string]string{}
	for _, s := range p.Source {
		s.ID = "project:" + s.ID
		remap[strings.TrimPrefix(s.ID, "project:")] = s.ID
		out.Source = append(out.Source, s)
	}
	for _, g := range p.Group {
		g.Source, g.Evidence = remap[g.Source], remap[g.Evidence]
		out.declared[len(out.Group)] = sev
		out.Group = append(out.Group, g)
	}
	out.reindex()
	return out
}

// Declared returns a project table that lists names as built-ins of the preset the
// static table cannot know (lint.builtin_names.extra): organization commands, MCP
// prompts, plugin skills. The names collide with a command or a skill; the outcome of
// a collision is unknown.
func Declared(preset string, names []string) *Table {
	if len(names) == 0 {
		return nil
	}
	t := &Table{Schema: SchemaVersion, Preset: preset, Severity: SeverityWarning}
	for _, kind := range []string{KindCommand, KindSkill} {
		t.Group = append(t.Group, Group{Kind: kind, UserCollision: Unknown, Match: MatchExact, Names: names})
	}
	t.reindex()
	return t
}

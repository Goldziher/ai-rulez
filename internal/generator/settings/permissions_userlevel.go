package settings

import (
	"strings"

	"github.com/Goldziher/ai-rulez/internal/generator/jsonmerge"
)

// Harnesses whose permission settings live in a user-level file only (their
// sidecars are `user_only`, so a project run writes nothing for them).
//
// Hermes: ~/.hermes/config.yaml `approvals.deny` (fnmatch patterns, case
// insensitive, checked before --yolo) and the top-level `command_allowlist`
// (exact text or fnmatch globs). Commands only.
//
// Kimi Code: ~/.kimi-code/config.toml `[[permission.rules]]` tables with
// `decision = "allow" | "deny" | "ask"` and a Claude-style `pattern` ("Bash",
// "Bash(rm -rf*)", "Read").
//
// Sources (read 2026-10-05): https://hermes-agent.nousresearch.com/docs/user-guide/security
// and https://moonshotai.github.io/kimi-code/en/configuration/config-files.html.
var (
	_ = registerPermissionDialect("hermes", buildHermes)
	_ = registerPermissionDialect("kimi", buildKimi)
)

func buildHermes(t *translation) ([]jsonmerge.OwnedKey, error) {
	t.askUnsupported()
	var allow, deny []any
	for _, e := range t.entries {
		if e.Action == ActionAsk {
			continue
		}
		patterns, why := hermesPatterns(e)
		if why != "" {
			t.drop(e, why)
			continue
		}
		for _, p := range patterns {
			if e.Action == ActionAllow {
				allow = append(allow, p)
			} else {
				deny = append(deny, p)
			}
		}
	}
	var keys []jsonmerge.OwnedKey
	if len(allow) > 0 {
		keys = append(keys, docArrayKey(t.cfg, t.docPath, []string{"command_allowlist"}, dedupe(allow)))
	}
	if len(deny) > 0 {
		keys = append(keys, docArrayKey(t.cfg, t.docPath, []string{"approvals", "deny"}, dedupe(deny)))
	}
	return keys, nil
}

func hermesPatterns(e permEntry) ([]string, string) {
	r := e.Rule
	if r.Kind != KindShell {
		return nil, "Hermes only has command allow and deny lists"
	}
	p := r.Shell()
	if e.Action == ActionAllow && strings.ContainsAny(p.Literal, "?[") {
		return nil, "`?` and `[` are literals in Claude rules but fnmatch syntax here, which would widen the rule"
	}
	switch p.Kind {
	case ShellAny:
		if e.Action == ActionAllow {
			return nil, "allowing every command is not generated"
		}
		return []string{"*"}, ""
	case ShellPrefix:
		return []string{p.Literal, p.Literal + " *"}, ""
	}
	return []string{p.Literal}, ""
}

func buildKimi(t *translation) ([]jsonmerge.OwnedKey, error) {
	var rules []any
	for _, e := range t.entries {
		patterns, why := kimiPatterns(e)
		if why != "" {
			t.drop(e, why)
			continue
		}
		for _, p := range patterns {
			rules = append(rules, map[string]any{"decision": string(e.Action), "pattern": p})
		}
	}
	if len(rules) == 0 {
		return nil, nil
	}
	return []jsonmerge.OwnedKey{docArrayKey(t.cfg, t.docPath, []string{"permission", "rules"}, dedupe(rules))}, nil
}

func kimiPatterns(e permEntry) ([]string, string) {
	r := e.Rule
	switch {
	case r.Kind == KindShell:
		p := r.Shell()
		switch p.Kind {
		case ShellAny:
			return []string{"Bash"}, ""
		case ShellPrefix:
			return []string{"Bash(" + p.Literal + ")", "Bash(" + p.Literal + " *)"}, ""
		}
		return []string{"Bash(" + p.Literal + ")"}, ""
	case r.Kind == KindRead && r.Bare:
		return []string{"Read"}, ""
	}
	return nil, "only Bash rules and a bare Read rule are documented for Kimi Code"
}

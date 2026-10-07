package settings

import (
	"github.com/Goldziher/ai-rulez/v5/internal/generator/jsonmerge"
)

// Gemini CLI: .gemini/settings.json `tools.allowed` (skip the confirmation) and
// `tools.exclude` (block). Shell entries are `run_shell_command(<prefix>)`.
// Project-level policy files (.gemini/policies/*.toml) are documented as
// non-functional ("The Workspace tier ... is currently non-functional"), so the
// legacy settings keys are the only project-level surface. Gemini has no ask list
// in settings.json: whatever is not allowed already prompts.
//
// Sources: https://geminicli.com/docs/reference/configuration/ and
// https://geminicli.com/docs/reference/policy-engine/ (read 2026-10-05).
var _ = registerPermissionDialect("gemini", buildGemini)

const geminiShell = "run_shell_command"

// geminiTools maps a bare rule to Gemini's tool names; deny lists every name the
// capability may go by, allow only the primary one.
var geminiTools = map[string]struct{ allow, deny []string }{
	toolRead:      {[]string{nativeReadFile}, []string{nativeReadFile, "read_many_files"}},
	toolEdit:      {[]string{"replace", nativeWriteFile}, []string{"replace", nativeWriteFile}},
	toolWrite:     {[]string{nativeWriteFile}, []string{nativeWriteFile}},
	toolWebFetch:  {[]string{nativeWebFetch}, []string{nativeWebFetch}},
	toolWebSearch: {[]string{"google_web_search"}, []string{"google_web_search"}},
}

func buildGemini(t *translation) ([]jsonmerge.OwnedKey, error) {
	var allowed, excluded []any
	t.askUnsupported()
	for ix := range t.entries {
		e := t.entries[ix]
		if e.Action == ActionAsk {
			continue
		}
		names, why := geminiNames(e)
		if why != "" {
			t.drop(e, why)
			continue
		}
		for _, n := range names {
			if e.Action == ActionAllow {
				allowed = append(allowed, n)
			} else {
				excluded = append(excluded, n)
			}
		}
	}
	var keys []jsonmerge.OwnedKey
	if len(allowed) > 0 {
		keys = append(keys, docArrayKey(t.cfg, t.docPath, []string{keyTools, "allowed"}, allowed))
	}
	if len(excluded) > 0 {
		keys = append(keys, docArrayKey(t.cfg, t.docPath, []string{keyTools, "exclude"}, excluded))
	}
	return keys, nil
}

func geminiNames(e permEntry) (names []string, why string) {
	r := e.Rule
	if r.Kind == KindShell {
		p := r.Shell()
		switch {
		case p.Kind == ShellAny:
			return []string{geminiShell}, ""
		case p.Kind == ShellPrefix && e.Action == ActionAllow:
			// Gemini documents plain prefix matching, so `git` would also approve `gitk`;
			// the trailing space ends the allowed prefix at a word boundary.
			return []string{geminiShell + "(" + p.Literal + " )"}, ""
		case p.Kind == ShellPrefix, p.Kind == ShellExact && e.Action != ActionAllow:
			// A prefix is stricter than an exact command, which is fine for a deny
			// but would widen an allow.
			return []string{geminiShell + "(" + p.Literal + ")"}, ""
		case p.Kind == ShellExact:
			return nil, "Gemini matches command prefixes, so an exact-command allow would be widened"
		}
		return nil, "Gemini shell rules are command prefixes; wildcards inside a command cannot be expressed"
	}
	tool, ok := geminiTools[r.Tool]
	if !ok {
		return nil, "settings.json has no equivalent of " + r.Tool + " rules"
	}
	if !r.Bare {
		return nil, "settings.json can only allow or block a whole tool, not " + r.Specifier
	}
	if e.Action == ActionAllow {
		return tool.allow, ""
	}
	return tool.deny, ""
}

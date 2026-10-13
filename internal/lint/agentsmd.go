package lint

import (
	"path"
	"path/filepath"
	"strings"
)

// The CLAUDE.md shadow check (AR9Q0). It is off by default: only when a project
// sets [lint] agents_md_only = true does it assert that AGENTS.md is canonical
// for Claude Code, so only then is a stray CLAUDE.md a problem.
const CodeClaudeMDShadow = "AR9Q0"

// agentsMDImport is the whole body of the generated CLAUDE.md shim: Claude Code
// reads the shared AGENTS.md through this import.
const agentsMDImport = "@AGENTS.md"

// configLayouts returns the places a nested root's configuration can live.
func configLayouts() []string {
	return []string{".agents/ai-rulez.toml", ".ai-rulez/config.toml", ".config/ai-rulez/config.toml"}
}

func registerAgentsMD(s *ruleSet) {
	s.addRules(
		RuleInfo{CodeClaudeMDShadow, "claude-md-shadows-agents-md", SeverityOff,
			"AGENTS.md is canonical for Claude Code, but a tracked CLAUDE.md, .claude/CLAUDE.md or CLAUDE.local.md is not the generated shim and shadows the nearest AGENTS.md"},
	)
	s.addDocs(map[string]RuleDoc{
		CodeClaudeMDShadow: {
			Why:  "Claude Code loads the nearest CLAUDE.md in preference to AGENTS.md, so a hand-authored CLAUDE.md silently replaces the instructions ai-rulez generated; CLAUDE.local.md shadows it for one developer only.",
			Bad:  "A hand-written CLAUDE.md next to the generated AGENTS.md",
			Good: "Keep the generated CLAUDE.md shim (whose only body is @AGENTS.md), or list an intentional island in lint.agents_md_only_allow",
		},
	})
	SetAnalyzer(CodeClaudeMDShadow, AnalyzerTraps, ScopeFile)
	s.addRunCheck(checkClaudeMDShadow, AnalyzerTraps)
}

// checkClaudeMDShadow reports every tracked CLAUDE.md, .claude/CLAUDE.md and
// CLAUDE.local.md that is not the generated "@AGENTS.md" shim and shadows the
// AGENTS.md Claude Code would otherwise load. AR9Q0 is off unless
// [lint] agents_md_only is set (or a [lint.severity] entry enables it).
//
// The file list is the tracked tree, so a hand-authored CLAUDE.md that only
// exists in the working tree is not reported. Each nested root owns the files
// under it, so a --recursive run reports a nested root's files from that root,
// not from the top.
func checkClaudeMDShadow(r *runner) {
	if r.sev[CodeClaudeMDShadow] == SeverityOff {
		return
	}
	allow := compileGlobs(r.lc.AgentsMDOnlyAllow)
	for _, rel := range r.tree.Paths() {
		if !isClaudeMDFile(rel) || !r.belongsToRoot(rel) {
			continue
		}
		abs := filepath.Join(r.tree.Top, filepath.FromSlash(rel))
		if r.claudeMDAllowed(abs, rel, allow) {
			continue
		}
		if data, ok := r.fileContent(abs); ok && isAgentsMDShim(data) {
			continue
		}
		shadowed, _ := r.nearestAgentsMD(rel) // "" falls back to the canonical name
		if shadowed == "" {
			shadowed = "AGENTS.md"
		}
		r.add(CodeClaudeMDShadow, abs, 1,
			"%s shadows %s for Claude Code: AGENTS.md is canonical (lint.agents_md_only), so this file changes what Claude Code loads. Remove it, keep only the generated shim whose body is @AGENTS.md, or list it in lint.agents_md_only_allow",
			path.Base(rel), shadowed)
	}
}

// isClaudeMDFile reports whether rel is a file Claude Code loads as project
// memory: CLAUDE.md (in any directory, including .claude/) or CLAUDE.local.md.
func isClaudeMDFile(rel string) bool {
	switch path.Base(rel) {
	case "CLAUDE.md", "CLAUDE.local.md":
		return true
	}
	return false
}

// isAgentsMDShim reports whether content is the generated CLAUDE.md shim: the
// "@AGENTS.md" import as its whole body, after an optional banner. A file that
// only imports AGENTS.md does not shadow it.
func isAgentsMDShim(content string) bool {
	if i := strings.Index(content, "-->"); i >= 0 {
		content = content[i+len("-->"):]
	}
	return strings.TrimSpace(content) == agentsMDImport
}

// belongsToRoot reports whether rel is in this root's scope: under the root and
// not under a nested root that owns it (a directory below the root with its own
// configuration). A --recursive run then reports each root's files once.
func (r *runner) belongsToRoot(rel string) bool {
	sub, ok := r.underRoot(rel)
	if !ok {
		return false
	}
	for dir := path.Dir(sub); dir != "." && dir != ""; dir = path.Dir(dir) {
		for _, layout := range configLayouts() {
			if r.tree.Exists(path.Join(r.baseRel, dir, layout)) {
				return false
			}
		}
	}
	return true
}

// claudeMDAllowed reports whether abs (repo-relative rel) matches an
// agents_md_only_allow glob, taken relative to the config directory or the repo
// root.
func (r *runner) claudeMDAllowed(abs, rel string, allow []globMatcher) bool {
	if len(allow) == 0 {
		return false
	}
	cands := []string{rel}
	if crel := r.configRel(abs); crel != "" {
		cands = append(cands, crel)
	}
	for _, c := range cands {
		for _, g := range allow {
			if g.match(c) {
				return true
			}
		}
	}
	return false
}

// nearestAgentsMD returns the AGENTS.md at rel's directory or the closest
// ancestor, the file Claude Code would load if rel were not there.
func (r *runner) nearestAgentsMD(rel string) (string, bool) {
	for dir := path.Dir(rel); ; dir = path.Dir(dir) {
		cand := path.Join(dir, "AGENTS.md")
		if r.tree.Exists(cand) {
			return cand, true
		}
		if dir == "." || dir == "" {
			return "", false
		}
	}
}

package lint

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	procrunner "github.com/Goldziher/ai-rulez/v5/internal/runner"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func (r *runner) configFilePath() string {
	if r.cfg.ConfigFile == "" {
		return ""
	}
	p, _ := filepath.Abs(filepath.Join(r.cfg.ConfigDir, r.cfg.ConfigFile)) //nolint:errcheck // display only
	return p
}

func (r *runner) checkMCP() {
	r.checkFrontmatterMCPCommands()
	path := r.configFilePath()
	if path == "" {
		return
	}
	var text []string
	if data, err := os.ReadFile(path); err == nil {
		text = strings.Split(string(data), "\n")
		r.docs[path] = doc{lines: text}
	}
	for i := range r.effectiveMCPServers() {
		s := &r.mcpEffective[i]
		n := s.Name
		if !s.IsEnabled() || s.GetTransport() != config.TransportStdio || s.Command == "" || strings.Contains(s.Command, "$") {
			continue
		}
		if r.commandResolves(s.Command) {
			continue
		}
		line := 1
		for i, l := range text {
			if strings.Contains(l, s.Command) {
				line = i + 1
				break
			}
		}
		r.add(CodeMCPCommandNotFound, path, line, "MCP server %q runs %q, which is not on PATH", n, s.Command)
	}
}

// checkFrontmatterMCPCommands reports an inline stdio server of an agent or
// skill whose command is not on PATH.
func (r *runner) checkFrontmatterMCPCommands() {
	for _, s := range r.frontmatterMCPServers() {
		if s.disabled || effectiveTransport(s) != transportStdio || s.command == "" || strings.Contains(s.command, "$") || r.commandResolves(s.command) {
			continue
		}
		r.add(CodeMCPCommandNotFound, s.file, s.line, "MCP server %q runs %q, which is not on PATH", s.name, s.command)
	}
}

func (r *runner) commandResolves(cmd string) bool {
	if strings.ContainsRune(cmd, filepath.Separator) || strings.HasPrefix(cmd, ".") {
		abs := cmd
		if !filepath.IsAbs(cmd) {
			abs = filepath.Join(r.rootAbs(), cmd)
			if _, err := os.Stat(abs); err != nil && r.tree.Explicit {
				abs = filepath.Join(r.tree.Top, cmd)
			}
		}
		info, err := os.Stat(abs)
		return err == nil && !info.IsDir()
	}
	_, err := procrunner.LookPath(cmd)
	return err == nil
}

var projectVarRe = regexp.MustCompile(`(?:\$\{CLAUDE_PROJECT_DIR\}|\$CLAUDE_PROJECT_DIR)"?/([^\s"';&|)]+)`)

type hookFile struct {
	Hooks map[string][]struct {
		Hooks []struct {
			Type    string   `json:"type"`
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"hooks"`
	} `json:"hooks"`
}

func (r *runner) checkHooks(baseAbs string) {
	path := filepath.Join(baseAbs, ".claude", "settings.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var hf hookFile
	if json.Unmarshal(data, &hf) != nil {
		return
	}
	r.docs[path] = doc{lines: strings.Split(string(data), "\n")}
	events := make([]string, 0, len(hf.Hooks))
	for e := range hf.Hooks {
		events = append(events, e)
	}
	sort.Strings(events)
	for _, event := range events {
		for _, group := range hf.Hooks[event] {
			for _, h := range group.Hooks {
				if h.Type != "" && h.Type != hookTypeCommand {
					continue
				}
				r.checkHookCommand(path, event, h.Command)
			}
		}
	}
}

func (r *runner) checkHookCommand(settings, event, command string) {
	r.checkLauncherScripts(settings, event, command)
	trimmed := strings.TrimLeft(command, `"' `)
	for _, m := range projectVarRe.FindAllStringSubmatchIndex(command, -1) {
		rel := command[m[2]:m[3]]
		line := hookLine(r.docs[settings], rel)
		found := ""
		for _, cand := range []string{rel, joinRel(r.baseRel, rel)} {
			if r.tree.Exists(cand) {
				found = cand
				break
			}
		}
		if found == "" {
			r.add(CodeHookMissing, settings, line, "%s hook runs %q, which does not exist", event, rel)
			continue
		}
		r.dep(settings, filepath.Join(r.tree.Top, filepath.FromSlash(found)))
		direct := m[0] == len(command)-len(trimmed)
		if exe, known := r.tree.Executable(found); direct && known && !exe {
			r.addFix(chmodFix(filepath.Join(r.tree.Top, filepath.FromSlash(found))), CodeHookNotExecutable, settings, line, "%s hook runs %q, which is not executable", event, rel)
		}
	}
}

func joinRel(base, rel string) string {
	if base == "" {
		return rel
	}
	return base + "/" + rel
}

func hookLine(d doc, needle string) int {
	for i, l := range d.lines {
		if strings.Contains(l, needle) {
			return i + 1
		}
	}
	return 1
}

// checkPluginDrift reports plugins whose content changed but whose version did
// not. A client that installed from a git-hosted marketplace keeps its cached
// copy until the version string changes (a plugin that declares no version is
// tracked by commit and is never reported).
func (r *runner) checkPluginDrift() {
	for _, d := range r.drift {
		changed := strings.Join(d.Changed, ", ")
		r.add(CodePluginVersionDrift, d.File, 1,
			"plugin %q changed since the baseline (%s) but its version is still %s; installs that cache the plugin keep the old copy until the version changes",
			d.Plugin, changed, d.Version)
	}
}

// checkEvals reports a skill that ships no eval cases. Cases live in the
// skill's own evals/ directory or in .ai-rulez/evals/<skill-name>/.
func (r *runner) checkEvals(it *item, d doc) {
	if r.sev[CodeEvalsMissing] == SeverityOff || it.kind != kindSkill || it.itemDir == "" {
		return
	}
	id := config.SkillID(it.cf)
	if r.evalsAllowed(id) {
		return
	}
	for _, dir := range []string{
		filepath.Join(it.itemDir, config.SkillKindEvals),
		filepath.Join(r.cfg.ConfigDir, config.EvalsDirName, id),
	} {
		if hasFiles(dir) {
			return
		}
	}
	r.add(CodeEvalsMissing, it.abs, d.lineOf("name", 1), "skill %q has no eval cases (add files under %s/ or %s/%s/)",
		id, config.SkillKindEvals, config.EvalsDirName, id)
}

func (r *runner) evalsAllowed(id string) bool {
	if r.lc.Evals == nil {
		return false
	}
	for _, pattern := range r.lc.Evals.Allow {
		if m, ok := newGlob(pattern); ok && m.match(id) {
			return true
		}
	}
	return false
}

// hasFiles reports whether dir holds at least one regular, non-hidden file.
func hasFiles(dir string) bool {
	found := false
	//nolint:errcheck // an unreadable or missing directory simply has no cases
	_ = filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type().IsRegular() && !strings.HasPrefix(entry.Name(), ".") {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

// checkSettingsConfig checks the [[hooks]] and [permissions] blocks of
// config.toml against the tree. The generated settings files may be gitignored,
// so the declaration is checked at its source: a hook script that is missing or
// not executable fails the hook at runtime, and an allow rule for a whole tool
// defeats the point of listing rules.
func (r *runner) checkSettingsConfig() {
	if len(r.cfg.Hooks) == 0 && r.cfg.Permissions.IsEmpty() {
		return
	}
	path := r.configFilePath()
	if path == "" {
		return
	}
	var text []string
	if data, err := os.ReadFile(path); err == nil {
		text = strings.Split(string(data), "\n")
		r.docs[path] = doc{lines: text}
	}
	lineOf := func(needle string) int {
		for i, l := range text {
			if strings.Contains(l, needle) {
				return i + 1
			}
		}
		return 1
	}
	for _, group := range r.cfg.Hooks {
		for _, action := range group.Hooks {
			if action.Script == "" {
				continue
			}
			rel := joinRel(r.baseRel, filepath.ToSlash(filepath.Clean(action.Script)))
			if !r.tree.Exists(rel) {
				r.add(CodeHookSourceMissing, path, lineOf(action.Script), "%s hook runs %q, which does not exist", group.Event, action.Script)
				continue
			}
			if exe, known := r.tree.Executable(rel); known && !exe {
				r.addFix(chmodFix(filepath.Join(r.tree.Top, filepath.FromSlash(rel))), CodeHookSourceNotExec, path, lineOf(action.Script), "%s hook runs %q, which is not executable", group.Event, action.Script)
			}
		}
	}
	for _, rule := range r.cfg.Permissions.OverbroadAllowRules() {
		r.add(CodePermissionOverbroad, path, lineOf(rule), "permissions.allow %q permits every call of the tool; name the commands or paths it may run", rule)
	}
}

// validateBudgetAndRisk checks [lint.ratchet] and [lint.risk].
func validateBudgetAndRisk(lc *config.LintConfig) []string {
	var problems []string
	if _, ok := LookupProfile(lc.Profile); !ok {
		problems = append(problems, fmt.Sprintf("lint.profile: unknown profile %q (use %s)", lc.Profile, strings.Join(ProfileNames(), ", ")))
	}
	for key, limit := range lc.Ratchet {
		if _, ok := lookupRule(key); !ok {
			problems = append(problems, fmt.Sprintf("lint.ratchet: unknown rule %q", key))
		}
		if limit < 0 {
			problems = append(problems, fmt.Sprintf("lint.ratchet.%s: %d is negative", key, limit))
		}
	}
	if lc.Risk != nil {
		for _, w := range []struct {
			name string
			v    *int
		}{{string(SeverityError), lc.Risk.Error}, {string(SeverityWarning), lc.Risk.Warning}, {string(SeverityInfo), lc.Risk.Info}} {
			if w.v != nil && *w.v < 0 {
				problems = append(problems, fmt.Sprintf("lint.risk.%s: %d is negative", w.name, *w.v))
			}
		}
	}
	return problems
}

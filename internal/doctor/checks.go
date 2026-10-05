package doctor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/docmerge"
	"github.com/Goldziher/ai-rulez/v5/internal/includes"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/schema"
	"github.com/samber/oops"
)

// errorFinding turns an error into a finding, keeping an oops hint and listing
// the validation errors an oops error carries.
func errorFinding(check string, sev Severity, err error, path string) []Finding {
	hint := ""
	var lines []string
	if o, ok := oops.AsOops(err); ok {
		hint = firstLine(o.Hint())
		if list, ok := o.Context()["errors"].([]string); ok {
			lines = list
		}
	}
	msg := firstLine(err.Error())
	if len(lines) == 0 {
		return []Finding{{Check: check, Severity: sev, Message: msg, Path: path, Hint: hint}}
	}
	out := make([]Finding, 0, len(lines))
	for _, l := range lines {
		out = append(out, Finding{Check: check, Severity: sev, Message: l, Path: path, Hint: hint})
	}
	return out
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return line
}

// checkConfig: the configuration loads, matches the schema and validates.
func checkConfig(_ context.Context, s *state) []Finding {
	if s.loadErr != nil {
		return errorFinding(CheckConfig, SeverityError, s.loadErr, "")
	}
	cfg := s.cfg
	var out []Finding
	if !cfg.IsV3() && cfg.ConfigFile != "" {
		path := filepath.Join(cfg.ConfigDir, cfg.ConfigFile)
		if err := schema.ValidateFile(path); err != nil {
			for _, f := range errorFinding(CheckConfig, SeverityError, err, path) {
				f.Message = strings.TrimPrefix(f.Message, "- ")
				// The presets check explains an unknown preset with a suggestion.
				if len(unknownPresets(cfg)) > 0 && strings.HasPrefix(f.Message, "presets.") {
					continue
				}
				out = append(out, f)
			}
		}
	}
	if err := cfg.Validate(); err != nil {
		// The presets check explains an unknown preset with a suggestion.
		if len(unknownPresets(cfg)) > 0 && strings.Contains(err.Error(), "unknown built-in preset") {
			return out
		}
		out = append(out, errorFinding(CheckConfig, SeverityError, err, "")...)
	}
	return out
}

// removedPresets maps a preset name that no longer exists to what replaced it.
// An empty replacement means there is none.
var removedPresets = map[string]string{
	"windsurf":     "devin",
	"continue-dev": "",
}

type unknownPreset struct {
	Name        string
	Replacement string // from removedPresets
	Removed     bool
	Suggestion  string // nearest built-in name, when close enough
}

// unknownPresets lists the configured built-in presets that do not exist.
func unknownPresets(cfg *config.Config) []unknownPreset {
	known := config.AllPresetNames()
	isKnown := make(map[string]bool, len(known))
	for _, n := range known {
		isKnown[n] = true
	}
	var out []unknownPreset
	for i := range cfg.Presets {
		p := &cfg.Presets[i]
		if !p.IsBuiltIn() || isKnown[p.BuiltIn] {
			continue
		}
		u := unknownPreset{Name: p.BuiltIn}
		u.Replacement, u.Removed = removedPresets[p.BuiltIn]
		if !u.Removed {
			u.Suggestion = nearest(p.BuiltIn, known)
		}
		out = append(out, u)
	}
	return out
}

// checkPresets: every built-in preset name exists; removed ones name their
// replacement and unknown ones the closest valid name.
func checkPresets(_ context.Context, s *state) []Finding {
	if s.cfg == nil {
		return nil
	}
	var out []Finding
	for _, u := range unknownPresets(s.cfg) {
		f := Finding{Check: CheckPresets, Severity: SeverityError}
		switch {
		case u.Removed && u.Replacement != "":
			f.Message = fmt.Sprintf("preset %q was removed", u.Name)
			f.Hint = fmt.Sprintf("use %q instead", u.Replacement)
		case u.Removed:
			f.Message = fmt.Sprintf("preset %q was removed and has no replacement", u.Name)
			f.Hint = "drop it from presets"
		case u.Suggestion != "":
			f.Message = fmt.Sprintf("unknown preset %q", u.Name)
			f.Hint = fmt.Sprintf("did you mean %q?", u.Suggestion)
		default:
			f.Message = fmt.Sprintf("unknown preset %q", u.Name)
			f.Hint = "available presets: " + strings.Join(config.AllPresetNames(), ", ")
		}
		out = append(out, f)
	}
	return out
}

// nearest returns the candidate closest to name by edit distance, or "" when
// none is close enough to be a plausible typo.
func nearest(name string, candidates []string) string {
	limit := max(2, len(name)/3)
	best, bestDist := "", limit+1
	for _, c := range candidates {
		if d := levenshtein(name, c); d < bestDist {
			best, bestDist = c, d
		}
	}
	return best
}

// levenshtein is the edit distance between a and b.
func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur := make([]int, len(rb)+1)
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(rb)]
}

// checkMCPEnv: ${VAR} placeholders in MCP env and headers resolve.
func checkMCPEnv(_ context.Context, s *state) []Finding {
	if s.cfg == nil {
		return nil
	}
	missing, err := generator.NewGenerator(s.cfg).MissingMCPEnv(s.opts.Profile)
	if err != nil {
		return errorFinding(CheckMCPEnv, SeverityWarning, err, "")
	}
	out := make([]Finding, 0, len(missing))
	for _, m := range missing {
		out = append(out, Finding{
			Check:    CheckMCPEnv,
			Severity: SeverityWarning,
			Message:  fmt.Sprintf("MCP server %q %s %s references ${%s}, which is not set", m.Server, m.Field, m.Key, m.Var),
			Hint:     fmt.Sprintf("export %s, add it to .env, or pass --env %s=VALUE to generate", m.Var, m.Var),
		})
	}
	return out
}

// checkDrift: generated files match what the sources render (generate --check).
func checkDrift(_ context.Context, s *state) []Finding {
	if s.cfg == nil {
		return nil
	}
	if unresolvedSources(s.cfg) {
		// doctor never fetches, so what the includes contribute is unknown and a
		// render would report drift that is not there.
		s.renderFailed = true
		return []Finding{{
			Check: CheckDrift, Severity: SeverityInfo,
			Message: "drift not checked: includes and installed skills are not resolved (doctor never uses the network)",
			Hint:    "run `ai-rulez generate --check` to compare against the resolved content",
		}}
	}
	drift, err := generator.NewGenerator(s.cfg).CheckDrift(s.opts.Profile)
	if err != nil {
		s.renderFailed = true
		if o, ok := oops.AsOops(err); ok && o.Context()["unresolved"] != nil {
			return []Finding{{
				Check: CheckDrift, Severity: SeverityInfo,
				Message: "drift not checked: MCP placeholders are unresolved (see the mcp-env findings)",
			}}
		}
		f := errorFinding(CheckDrift, SeverityError, err, "")
		f[0].Message = "cannot render outputs: " + f[0].Message
		return f
	}
	out := make([]Finding, 0, len(drift))
	for _, d := range drift {
		out = append(out, Finding{
			Check: CheckDrift, Severity: SeverityWarning, Path: d.Path,
			Message: "generated file is " + string(d.Kind),
			Hint:    "run `ai-rulez generate`",
		})
	}
	return out
}

// unresolvedSources reports whether the configuration declares content doctor
// does not load: includes and installed skills.
func unresolvedSources(cfg *config.Config) bool {
	return len(cfg.Includes) > 0 || len(cfg.InstalledSkills) > 0
}

// checkGitignore: outputs generate wants git to ignore are ignored.
func checkGitignore(_ context.Context, s *state) []Finding {
	if s.cfg == nil || s.renderFailed {
		return nil
	}
	missing, err := generator.NewGenerator(s.cfg).UnignoredOutputs(s.opts.Profile)
	if err != nil {
		return errorFinding(CheckGitignore, SeverityWarning, err, "")
	}
	out := make([]Finding, 0, len(missing))
	for _, pattern := range missing {
		out = append(out, Finding{
			Check: CheckGitignore, Severity: SeverityWarning, Path: pattern,
			Message: "generated output is not ignored by git",
			Hint:    "run `ai-rulez generate` to update the managed .gitignore block",
		})
	}
	return out
}

// checkDocuments: shared settings documents ai-rulez merges into still parse.
func checkDocuments(_ context.Context, s *state) []Finding {
	if s.cfg == nil {
		return nil
	}
	var out []Finding
	for _, path := range generator.NewGenerator(s.cfg).MergedDocumentPaths() {
		if f, bad := documentFinding(s.cfg.BaseDir, path); bad {
			out = append(out, f)
		}
	}
	return out
}

// documentFinding reports a merged document that exists but does not parse.
// A missing file or an unmergeable extension is not a finding.
func documentFinding(baseDir, path string) (Finding, bool) {
	format, ok := docmerge.FormatFromPath(path)
	if !ok {
		return Finding{}, false
	}
	if _, err := os.Stat(path); err != nil {
		return Finding{}, false
	}
	if _, err := docmerge.Apply(path, format, nil); err != nil {
		rel, relErr := filepath.Rel(baseDir, path)
		if relErr != nil {
			rel = path
		}
		return Finding{
			Check: CheckDocuments, Severity: SeverityError, Path: filepath.ToSlash(rel),
			Message: fmt.Sprintf("shared %s document does not parse: %s", format, firstLine(err.Error())),
			Hint:    "fix the syntax; generate cannot merge into it until then",
		}, true
	}
	return Finding{}, false
}

// checkHooks: the scripts of [[hooks]] exist and are executable. It stats the
// paths itself, so the result does not depend on git state or on lint rule
// overrides (lint's AR504 and AR505 can be switched off).
func checkHooks(_ context.Context, s *state) []Finding {
	if s.cfg == nil || len(s.cfg.Hooks) == 0 {
		return nil
	}
	var out []Finding
	seen := map[string]bool{}
	for _, group := range s.cfg.Hooks {
		for _, action := range group.Hooks {
			script := action.Script
			if script == "" || seen[script] {
				continue
			}
			seen[script] = true
			path := script
			if !filepath.IsAbs(path) {
				path = filepath.Join(s.cfg.BaseDir, filepath.FromSlash(script))
			}
			info, err := os.Stat(path)
			switch {
			case err != nil:
				out = append(out, Finding{
					Check: CheckHooks, Severity: SeverityError, Path: script,
					Message: fmt.Sprintf("%s hook runs %q, which does not exist", group.Event, script),
				})
			case info.IsDir():
				out = append(out, Finding{
					Check: CheckHooks, Severity: SeverityError, Path: script,
					Message: fmt.Sprintf("%s hook runs %q, which is a directory", group.Event, script),
				})
			case runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0:
				out = append(out, Finding{
					Check: CheckHooks, Severity: SeverityError, Path: script,
					Message: fmt.Sprintf("%s hook runs %q, which is not executable", group.Event, script),
					Hint:    "chmod +x " + script,
				})
			}
		}
	}
	return out
}

// hintRunLock is the hint of a stale lock.
const hintRunLock = "run `ai-rulez lock`"

// checkLock: ai-rulez.lock matches the remote includes and installed skills and,
// when it pins authored content, the sources on disk. The comparison is offline
// and reads sources only; generated outputs, skill sources and served skills are
// compared by `ai-rulez lock --check`.
func checkLock(ctx context.Context, s *state) []Finding {
	if s.cfg == nil {
		return nil
	}
	cfg, err := s.opts.Load(ctx, config.WithoutLocal(), config.WithoutRemote())
	if err != nil {
		return errorFinding(CheckLock, SeverityWarning, err, "")
	}
	lock, err := lockfile.Load(cfg.ConfigDir)
	if err != nil {
		return errorFinding(CheckLock, SeverityError, err, lockfile.FileName)
	}
	var out []Finding
	problems, _ := includes.CheckLock(cfg, lock)
	if len(problems) > 0 {
		out = append(out, Finding{
			Check: CheckLock, Severity: SeverityWarning, Path: lockfile.FileName,
			Message: "does not match the configuration: " + strings.ReplaceAll(strings.TrimSpace(includes.FormatProblems(problems)), "\n", "; "),
			Hint:    hintRunLock,
		})
	}
	return append(out, lockContentFindings(cfg, lock)...)
}

// lockContentFindings compares the authored sources with the content pins of the
// lock. Drift is an error when [lock] enforce is set (strict validation reports
// it as AR981), a warning otherwise.
func lockContentFindings(cfg *config.Config, lock *lockfile.File) []Finding {
	severity := SeverityWarning
	if cfg.LockEnforced() {
		severity = SeverityError
	}
	if lock == nil {
		if cfg.LockEnforced() {
			return []Finding{{Check: CheckLock, Severity: severity, Path: lockfile.FileName,
				Message: "does not exist but [lock] enforce = true", Hint: hintRunLock}}
		}
		return nil
	}
	if !lock.HasContentPins() {
		if cfg.LockEnforced() {
			return []Finding{{Check: CheckLock, Severity: severity, Path: lockfile.FileName,
				Message: "has no content pins and [lock] enforce = true", Hint: hintRunLock}}
		}
		return nil
	}
	snap, err := contentlock.Compute(cfg, contentlock.Options{
		Scope: cfg.LockScope(), IncludeOutputs: cfg.LockIncludeOutputs(), Profile: lock.Profile, SourcesOnly: true,
	})
	if err != nil {
		return errorFinding(CheckLock, SeverityWarning, err, lockfile.FileName)
	}
	changes := contentlock.Compare(lock, snap).Changes
	if len(changes) == 0 {
		return nil
	}
	lines := make([]string, 0, len(changes))
	for i := range changes {
		lines = append(lines, changes[i].Line())
	}
	return []Finding{{
		Check: CheckLock, Severity: severity, Path: lockfile.FileName,
		Message: "authored content differs from its pins: " + strings.Join(lines, "; "),
		Hint:    "review with `ai-rulez lock --diff`, then run `ai-rulez lock`",
	}}
}

// presetBinaries maps a preset to the executables of its tool; any one on PATH
// counts. Presets that are not listed are skipped: either they have no CLI or
// the binary name is not known with certainty.
var presetBinaries = map[string][]string{
	"claude":   {"claude"},
	"codex":    {"codex"},
	"gemini":   {"gemini"},
	"opencode": {"opencode"},
	"amp":      {"amp"},
	"copilot":  {"copilot"},
	"cline":    {"cline"},
	"devin":    {"devin"},
	"cursor":   {"cursor-agent", "cursor"},
	"junie":    {"junie"},
}

// checkTools: the tool behind each preset is installed (information only; the
// output is generated either way).
func checkTools(_ context.Context, s *state) []Finding {
	if s.cfg == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []Finding
	for i := range s.cfg.Presets {
		name := s.cfg.Presets[i].BuiltIn
		bins, ok := presetBinaries[name]
		if !ok || seen[name] {
			continue
		}
		seen[name] = true
		if anyOnPath(s.opts.LookPath, bins) {
			continue
		}
		out = append(out, Finding{
			Check: CheckTools, Severity: SeverityInfo,
			Message: fmt.Sprintf("preset %q: %s not found on PATH; its files are still generated", name, strings.Join(bins, " or ")),
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Message < out[j].Message })
	return out
}

func anyOnPath(look func(string) (string, error), bins []string) bool {
	for _, b := range bins {
		if _, err := look(b); err == nil {
			return true
		}
	}
	return false
}

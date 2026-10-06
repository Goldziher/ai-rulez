package lint

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	toml "github.com/pelletier/go-toml/v2"

	"github.com/Goldziher/ai-rulez/v5/internal/harnesslimits"
)

// The frontmatter-first and json-key-required-if predicates, the Kiro traps
// (AR9C5, AR9C6), project-defined trap rows (AR9CA) and the safe fix of a
// misspelt key.
//
// The traps read the files on disk, except for a file a run would write, which they
// read from the plan (PlannedFiles): the generator imports this package, so a trap
// cannot call it, and the command line hands the generator's plan in as an interface.
// A generated file is judged as the harness will see it whether or not `generate` ran.

// Codes of the Kiro traps and of project rows.
const (
	CodeKiroAgentSteering  = "AR9C5"
	CodeKiroSteeringFirst  = "AR9C6"
	CodeProjectTrap        = "AR9CA"
	maxProjectTrapFiles    = 64
	maxProjectTrapFileSize = 256 << 10
	maxFrontmatterScan     = 10
)

func init() {
	registerRules(
		RuleInfo{CodeKiroAgentSteering, "kiro-agent-steering-not-loaded", SeverityWarning, "a Kiro custom agent file has no resources while .kiro/steering holds steering files, so the agent never loads them"},
		RuleInfo{CodeKiroSteeringFirst, "kiro-steering-frontmatter-not-first", SeverityWarning, "a Kiro steering file has its inclusion frontmatter after a blank line or other text, so Kiro does not read it"},
		RuleInfo{CodeProjectTrap, "project-trap", SeverityWarning, "a trap row of the project (.ai-rulez/traps/*.toml) matched a file, or a row is invalid"},
	)
	registerRuleDocs(map[string]RuleDoc{
		CodeKiroAgentSteering: {
			Why:  "Kiro does not include steering files in a custom agent on its own; the agent must list them in its resources, so the steering context is missing without an error.",
			Bad:  "`.kiro/agents/review.json` without `resources`, next to `.kiro/steering/style.md`",
			Good: "`\"resources\": [\"file://.kiro/steering/**/*.md\"]` in the agent",
		},
		CodeKiroSteeringFirst: {
			Why:  "Kiro reads the inclusion setting only when it is the first content of the steering file, so a blank line or text before the opening `---` leaves the file on its default inclusion.",
			Bad:  "A steering file that starts with an empty line, then `---` and `inclusion: manual`",
			Good: "Start the file with `---` on the first byte",
		},
		CodeProjectTrap: {
			Why:  "A project can record its own traps as rows in `.ai-rulez/traps/*.toml`, with the same closed predicate vocabulary as the built-in table; this code reports a row's match or a row that cannot be used.",
			Bad:  "A row whose predicate kind is not in the vocabulary, or a file that matches the row",
			Good: "Fix the file the row names, or correct the row",
		},
	})
}

// evalFirst flags a file whose frontmatter does not start on the first byte: the
// first "---" line is within the first lines but not the first, a closing "---"
// follows, and the block holds the Key line. Without the Key condition a
// horizontal rule would match.
func (p TrapPredicate) evalFirst(content []byte) []trapHit {
	if bytes.HasPrefix(content, []byte("---")) {
		return nil
	}
	lines := strings.Split(string(content), "\n")
	open := -1
	for i := 0; i < len(lines) && i < maxFrontmatterScan; i++ {
		if strings.TrimPrefix(strings.TrimRight(lines[i], "\r \t"), "\xef\xbb\xbf") == "---" {
			open = i
			break
		}
	}
	if open < 0 {
		return nil
	}
	hasKey := p.Key == ""
	for _, l := range lines[open+1:] {
		trimmed := strings.TrimRight(l, "\r \t")
		if trimmed == "---" {
			if !hasKey {
				return nil
			}
			return []trapHit{{line: 1, detail: fmt.Sprintf("the frontmatter starts on line %d", open+1)}}
		}
		if p.Key != "" && strings.HasPrefix(trimmed, p.Key+":") {
			hasKey = true
		}
	}
	return nil
}

// evalJSONKey flags a JSON object that lacks Key (or holds it empty) while a
// file ending in WhenSuffix exists in WhenDir. A file that is not a JSON object
// is not this predicate's business.
func (p TrapPredicate) evalJSONKey(content []byte, env trapEnv) []trapHit {
	var doc map[string]any
	if json.Unmarshal(content, &doc) != nil || doc == nil {
		return nil
	}
	if v, ok := doc[p.Key]; ok && !emptyJSON(v) {
		return nil
	}
	if env.exists == nil || !env.exists(p.WhenDir, p.WhenSuffix) {
		return nil
	}
	return []trapHit{{line: 1, detail: fmt.Sprintf("no %q while %s/*%s exists", p.Key, p.WhenDir, p.WhenSuffix)}}
}

func emptyJSON(v any) bool {
	switch t := v.(type) {
	case nil:
		return true
	case []any:
		return len(t) == 0
	case map[string]any:
		return len(t) == 0
	case string:
		return t == ""
	}
	return false
}

// packagePrefix is the part of rel before the scope directory: "" at the lint
// root, "pkg/" in a nested package.
func (s TrapScope) packagePrefix(rel string) string {
	dir := strings.Trim(s.Dir, "/")
	if dir == "" || strings.HasPrefix(rel, dir+"/") {
		return ""
	}
	if i := strings.Index(rel, "/"+dir+"/"); i >= 0 {
		return rel[:i+1]
	}
	return ""
}

// existsBeside answers trapEnv.exists for the package that holds rel.
func (r *runner) existsBeside(rel string, scope TrapScope) func(dir, suffix string) bool {
	return func(dir, suffix string) bool {
		if dir == "" || filepath.IsAbs(dir) || slices.Contains(strings.Split(filepath.ToSlash(dir), "/"), "..") {
			return false
		}
		abs := filepath.Join(r.tree.Top, filepath.FromSlash(path.Join(r.baseRel, scope.packagePrefix(rel), dir)))
		entries, err := os.ReadDir(abs)
		if err != nil {
			return false
		}
		for _, e := range entries {
			if e.Type().IsRegular() && strings.HasSuffix(strings.ToLower(e.Name()), strings.ToLower(suffix)) {
				return true
			}
		}
		return false
	}
}

func isRegularFile(abs string) bool {
	info, err := os.Lstat(abs)
	return err == nil && info.Mode().IsRegular()
}

// projectTrapKinds are the predicates a project row may use.
var projectTrapKinds = []string{
	predExtNotIn, predNameSuffixRequired, predFrontmatterEnum, predFrontmatterMissing,
	predKeyMisspelt, predSizeOver, predFrontmatterFirst, predJSONKeyRequiredIf,
}

// projectTraps loads .ai-rulez/traps/*.toml (once per run). Rows use the
// layout of the built-in table; code, source, quote and verified_on are
// optional. A file or row that cannot be used is reported as AR9CA and skipped.
func (r *runner) projectTraps() []Trap {
	if r.cfg == nil || r.cfg.ConfigDir == "" {
		return nil
	}
	dir := filepath.Join(r.cfg.ConfigDir, "traps")
	matches, err := filepath.Glob(filepath.Join(dir, "*.toml"))
	if err != nil || len(matches) == 0 {
		return nil
	}
	sort.Strings(matches)
	var out []Trap
	for i, file := range matches {
		abs, _ := filepath.Abs(file) //nolint:errcheck // display only
		if i >= maxProjectTrapFiles {
			r.add(CodeProjectTrap, abs, 1, "more than %d trap files; this one is not read", maxProjectTrapFiles)
			continue
		}
		rows, problems := parseProjectTraps(file)
		for _, p := range problems {
			r.add(CodeProjectTrap, abs, 1, "%s", p)
		}
		out = append(out, rows...)
	}
	return out
}

// parseProjectTraps reads one project trap file and validates every row.
func parseProjectTraps(file string) ([]Trap, []string) {
	info, err := os.Stat(file)
	if err != nil || !info.Mode().IsRegular() {
		return nil, []string{"trap file is not a regular file"}
	}
	if info.Size() > maxProjectTrapFileSize {
		return nil, []string{fmt.Sprintf("trap file is over %d bytes", maxProjectTrapFileSize)}
	}
	data, err := os.ReadFile(file) //nolint:gosec // a project file under the config directory
	if err != nil {
		return nil, []string{"read trap file: " + err.Error()}
	}
	var t trapTable
	dec := toml.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&t); err != nil {
		return nil, []string{"invalid trap file: " + err.Error()}
	}
	var rows []Trap
	var problems []string
	for i := range t.Trap {
		row := t.Trap[i]
		label := row.Name
		if label == "" {
			label = fmt.Sprintf("row %d", i+1)
		}
		if msg := row.validateProject(); msg != "" {
			problems = append(problems, fmt.Sprintf("trap %s: %s; the row is skipped", label, msg))
			continue
		}
		row.Project = true
		row.Code = CodeProjectTrap
		rows = append(rows, row)
	}
	return rows, problems
}

// validateProject returns why a project row cannot be used, or "".
func (t *Trap) validateProject() string {
	switch {
	case t.Name == "":
		return "name is required"
	case t.Harness == "":
		return "harness is required (a label shown in the finding)"
	case t.Message == "":
		return "message is required"
	case !slices.Contains(projectTrapKinds, t.Predicate.Kind):
		return fmt.Sprintf("predicate kind %q is not one of %s", t.Predicate.Kind, strings.Join(projectTrapKinds, ", "))
	}
	if strings.Contains(t.Scope.Dir, `\`) {
		return "scope.dir must use forward slashes"
	}
	if t.Scope.scopeDir() == "" && t.Scope.Suffix == "" {
		return "scope needs a dir or a suffix"
	}
	if filepath.IsAbs(t.Scope.Dir) || slices.Contains(strings.Split(filepath.ToSlash(t.Scope.Dir), "/"), "..") {
		return "scope.dir must be a relative path inside the project"
	}
	for _, k := range t.Scope.Kinds {
		if k != kindGenerated && k != kindHandwritten {
			return fmt.Sprintf("scope.kinds has %q (want %q or %q)", k, kindGenerated, kindHandwritten)
		}
	}
	switch t.Predicate.Kind {
	case predSizeOver:
		return t.validateProjectSize()
	case predJSONKeyRequiredIf:
		if t.Predicate.Key == "" || t.Predicate.WhenDir == "" {
			return "json-key-required-if needs key and when_dir"
		}
	case predFrontmatterEnum:
		if t.Predicate.Key == "" || len(t.Predicate.Allowed) == 0 {
			return "frontmatter-enum needs key and allowed"
		}
	case predExtNotIn:
		if len(t.Predicate.Allowed) == 0 {
			return "ext-not-in needs allowed"
		}
	case predKeyMisspelt:
		if len(t.Predicate.Canonical) == 0 {
			return "key-misspelt needs canonical"
		}
	case predNameSuffixRequired:
		if t.Predicate.Suffix == "" {
			return "name-suffix-required needs suffix"
		}
	case predFrontmatterMissing:
		if len(t.Predicate.Keys) == 0 {
			return "frontmatter-missing-all needs keys"
		}
	}
	return ""
}

func (t *Trap) validateProjectSize() string {
	p := &t.Predicate
	switch p.Measure {
	case measureFrontmatterChars:
		if len(p.Keys) == 0 {
			return "size-over with frontmatter-chars needs keys"
		}
	case measureFileChars, measureFileBytes:
	default:
		return fmt.Sprintf("size-over measure %q is not one of %s, %s, %s", p.Measure, measureFrontmatterChars, measureFileChars, measureFileBytes)
	}
	if p.Limit <= 0 {
		return "size-over needs limit > 0"
	}
	unit := "chars"
	if p.Measure == measureFileBytes {
		unit = "bytes"
	}
	p.limit = harnesslimits.Limit{Value: p.Limit, Unit: unit, Behavior: "over the project limit"}
	return ""
}

package verifiers

import (
	"bytes"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/verifiers/vspec"
	"github.com/pelletier/go-toml/v2"
	"github.com/samber/oops"
)

// Rule codes of the verifier family (docs/verifiers.md, docs/strict-validation.md).
const (
	CodeVerifierFailed    = "AR9H1"
	CodeVerifierInvalid   = "AR9H2"
	CodeVerifierDeadScope = "AR9H5"
	// CodeVerifierCommand is a command predicate that was refused (no --allow-exec,
	// an untrusted include) or did not run (not found, timed out).
	CodeVerifierCommand = "AR9H3"
	// CodeVerifierLLMSkipped is an llm predicate that was not evaluated.
	CodeVerifierLLMSkipped = "AR9H4"
	// CodeVerifierNoExamples is a verifier without self-test examples
	// ([verifiers_settings] require_examples).
	CodeVerifierNoExamples = "AR9H6"
)

const (
	// VerifiersDirName is the directory under the config directory that holds
	// verifier declaration files (`*.toml`, each with [[verifiers]] tables).
	VerifiersDirName = config.VerifiersDirName
	// maxSpecFileBytes bounds one declaration file.
	maxSpecFileBytes = 1 << 20
	// maxDepth is the deepest all/any/not nesting a predicate may have.
	maxDepth = 4
	// maxExampleBytes bounds one self-test fixture file.
	maxExampleBytes = 1 << 20

	inSameFile  = "same-file"
	inDiffAdded = "diff-added"
	inAnyFile   = "any-file"
)

var specIDRe = regexp.MustCompile(`^[a-z0-9._-]+$`)

// Spec is one verifier declared in `.ai-rulez/verifiers/*.toml`: a predicate
// tree attached to the rule, skill, agent or command it enforces.
type Spec struct {
	ID          string `toml:"id" json:"id"`
	Description string `toml:"description,omitempty" json:"description,omitempty"`
	// Exactly one of Rule, Skill, Agent and Command names the enforced item,
	// as `id` or `domain/id`.
	Rule    string `toml:"rule,omitempty" json:"rule,omitempty"`
	Skill   string `toml:"skill,omitempty" json:"skill,omitempty"`
	Agent   string `toml:"agent,omitempty" json:"agent,omitempty"`
	Command string `toml:"command,omitempty" json:"command,omitempty"`
	// Anchor is a heading in the target file; findings point at its line.
	Anchor   string `toml:"anchor,omitempty" json:"anchor,omitempty"`
	Severity string `toml:"severity,omitempty" json:"severity,omitempty"`
	Message  string `toml:"message,omitempty" json:"message,omitempty"`
	Fix      string `toml:"fix,omitempty" json:"fix,omitempty"`
	// WhenChanged scopes the verifier to changed files matching these globs.
	WhenChanged []string  `toml:"when_changed,omitempty" json:"when_changed,omitempty"`
	Exclude     []string  `toml:"exclude,omitempty" json:"exclude,omitempty"`
	Require     *Require  `toml:"require,omitempty" json:"require,omitempty"`
	Examples    []Example `toml:"examples,omitempty" json:"examples,omitempty"`

	// source is the declaration file, relative to the project root.
	source string
	// origin names the include the declaration arrived through; empty for the
	// project's own verifiers.
	origin string
}

// The predicate types live in vspec so config.toml can declare them inline.
type (
	Require        = vspec.Require
	RegexPred      = vspec.RegexPred
	FileExistsPred = vspec.FileExistsPred
	PairedPred     = vspec.PairedPred
	GlobCountPred  = vspec.GlobCountPred
	CommandPred    = vspec.CommandPred
	LLMPred        = vspec.LLMPred
	Example        = vspec.Example
)

// Problem is a declaration that cannot be used: reported as AR9H2 and never
// silently dropped.
type Problem struct {
	// ID is the verifier id, empty when the whole file is unusable.
	ID string
	// File is the declaration file relative to the project root.
	File    string
	Message string
}

type specFile struct {
	Verifiers []Spec `toml:"verifiers"`
}

// TargetKind returns which item kind the spec names and its id.
func (s *Spec) TargetKind() (kind, id string) {
	switch {
	case s.Rule != "":
		return "rule", s.Rule
	case s.Skill != "":
		return "skill", s.Skill
	case s.Agent != "":
		return "agent", s.Agent
	case s.Command != "":
		return "command", s.Command
	}
	return "", ""
}

// Source returns the declaration file relative to the project root.
func (s *Spec) Source() string { return s.source }

// inlineSource is the declaration file reported for a spec declared in config.toml.
const inlineSource = "config.toml"

// specFromConfig converts a spec-form [[verifiers]] entry; its name is the id.
func specFromConfig(v *config.VerifierConfig) Spec {
	return Spec{
		ID: v.Name, Description: v.Description, Rule: v.Rule, Skill: v.Skill, Agent: v.Agent, Command: v.Command,
		Anchor: v.Anchor, Severity: v.Severity, Message: v.Message, Fix: v.Fix, WhenChanged: v.WhenChanged,
		Exclude: v.Exclude, Require: v.Require, Examples: v.Examples, source: inlineSource,
	}
}

// LoadSpecs returns the usable specs and the problems of the spec-form
// [[verifiers]] of config.toml (declaration order) and of
// `<config dir>/verifiers/*.toml` (sorted by file, then declaration order),
// each validated against cfg. A symlink, a non-regular file, an oversized file,
// an unknown key and a duplicate id are problems, never silently skipped.
func LoadSpecs(cfg *config.Config) (specs []Spec, problems []Problem) {
	seen := map[string]string{}
	for i := range cfg.Verifiers {
		if v := &cfg.Verifiers[i]; !v.IsSpec() {
			seen[v.Name] = inlineSource
		}
	}
	for i := range cfg.Verifiers {
		v := &cfg.Verifiers[i]
		if !v.IsSpec() {
			continue
		}
		sp := specFromConfig(v)
		if msg := validateSpec(cfg, &sp); msg != "" {
			problems = append(problems, Problem{ID: sp.ID, File: inlineSource, Message: msg})
			continue
		}
		if prev, dup := seen[sp.ID]; dup {
			problems = append(problems, Problem{ID: sp.ID, File: inlineSource, Message: "duplicate verifier id (already declared in " + prev + ")"})
			continue
		}
		seen[sp.ID] = inlineSource
		specs = append(specs, sp)
	}
	dir := filepath.Join(cfg.ConfigDir, VerifiersDirName)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if !isMissing(err) {
			problems = append(problems, Problem{File: relTo(cfg.BaseDir, dir), Message: "cannot read directory: " + err.Error()})
		}
		imported, importProblems := loadImported(cfg, seen)
		return append(specs, imported...), append(problems, importProblems...)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".toml") {
			continue
		}
		file := filepath.Join(dir, e.Name())
		rel := relTo(cfg.BaseDir, file)
		parsed, perr := readSpecFile(file)
		if perr != nil {
			problems = append(problems, Problem{File: rel, Message: perr.Error()})
			continue
		}
		for i := range parsed {
			sp := parsed[i]
			sp.source = rel
			if msg := validateSpec(cfg, &sp); msg != "" {
				problems = append(problems, Problem{ID: sp.ID, File: rel, Message: msg})
				continue
			}
			if prev, dup := seen[sp.ID]; dup {
				problems = append(problems, Problem{ID: sp.ID, File: rel, Message: "duplicate verifier id (already declared in " + prev + ")"})
				continue
			}
			seen[sp.ID] = rel
			specs = append(specs, sp)
		}
	}
	imported, importProblems := loadImported(cfg, seen)
	return append(specs, imported...), append(problems, importProblems...)
}

// loadImported reads the verifier files that arrived through includes. They are
// validated like the project's own; one that uses the command predicate is
// refused unless its include is named in [verifiers_settings] trust_exec_from
// and pinned in the lock, so an include can never make this project run a program.
func loadImported(cfg *config.Config, seen map[string]string) (specs []Spec, problems []Problem) {
	if cfg.Content == nil {
		return nil, nil
	}
	settings := config.VerifiersSettings{}
	if cfg.VerifiersSettings != nil {
		settings = *cfg.VerifiersSettings
	}
	for _, f := range cfg.Content.ImportedVerifiers {
		src := "include:" + f.Include + "/" + VerifiersDirName + "/" + f.Name
		parsed, err := parseSpecs([]byte(f.Data))
		if err != nil {
			problems = append(problems, Problem{File: src, Message: err.Error()})
			continue
		}
		for i := range parsed {
			sp := parsed[i]
			sp.source, sp.origin = src, f.Include
			if msg := validateSpec(cfg, &sp); msg != "" {
				problems = append(problems, Problem{ID: sp.ID, File: src, Message: msg})
				continue
			}
			if msg := importRefusal(cfg, settings, &sp); msg != "" {
				problems = append(problems, Problem{ID: sp.ID, File: src, Message: msg})
				continue
			}
			if prev, dup := seen[sp.ID]; dup {
				problems = append(problems, Problem{ID: sp.ID, File: src, Message: "duplicate verifier id (already declared in " + prev + ")"})
				continue
			}
			seen[sp.ID] = src
			specs = append(specs, sp)
		}
	}
	return specs, problems
}

// importRefusal returns why an imported verifier may not be used, or "".
func importRefusal(cfg *config.Config, s config.VerifiersSettings, sp *Spec) string {
	if !usesCommand(sp.Require) {
		return ""
	}
	if !importTrusted(s, sp.origin) {
		return "uses the command predicate, which a verifier imported from include " + quote(sp.origin) +
			" may not: list the include in [verifiers_settings] trust_exec_from (it must also be pinned in ai-rulez.lock)"
	}
	if !includePinned(cfg, sp.origin) {
		return "include " + quote(sp.origin) + " is trusted to run commands but is not pinned in ai-rulez.lock: run `ai-rulez lock`"
	}
	return ""
}

// usesCommand reports whether a predicate tree contains a command predicate.
func usesCommand(r *Require) bool {
	if r == nil {
		return false
	}
	if r.Command != nil {
		return true
	}
	for _, kids := range [][]Require{r.All, r.Any} {
		for i := range kids {
			if usesCommand(&kids[i]) {
				return true
			}
		}
	}
	return usesCommand(r.Not)
}

// includePinned reports whether the lock pins the include: a commit and digest
// for a remote one, a tree digest (kind local-include) for a local path.
func includePinned(cfg *config.Config, name string) bool {
	lock, err := lockfile.Load(cfg.ConfigDir)
	if err != nil || lock == nil {
		return false
	}
	if lock.Find(lockfile.KindInclude, name) != nil {
		return true
	}
	for _, it := range lock.Item {
		if it.Kind == localIncludeKind && it.ID == name {
			return true
		}
	}
	return false
}

// localIncludeKind is the lock item kind of a local-path include (contentlock.KindLocalInclude).
const localIncludeKind = "local-include"

func relTo(base, p string) string {
	if rel, err := filepath.Rel(base, p); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(p)
}

// readSpecFile reads one declaration file without following a symlink.
func readSpecFile(file string) ([]Spec, error) {
	fi, err := os.Lstat(file)
	if err != nil {
		return nil, oops.Wrapf(err, "stat")
	}
	if !fi.Mode().IsRegular() {
		return nil, oops.Errorf("not a regular file (symlinks are not followed)")
	}
	f, err := os.Open(file)
	if err != nil {
		return nil, oops.Wrapf(err, "open")
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxSpecFileBytes+1))
	if err != nil {
		return nil, oops.Wrapf(err, "read")
	}
	if len(data) > maxSpecFileBytes {
		return nil, oops.Errorf("file is larger than %d KiB", maxSpecFileBytes>>10)
	}
	return parseSpecs(data)
}

// parseSpecs decodes the [[verifiers]] tables of one declaration file; an
// unknown key is an error.
func parseSpecs(data []byte) ([]Spec, error) {
	var parsed specFile
	dec := toml.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&parsed); err != nil {
		return nil, oops.Errorf("invalid TOML: %s", strings.ReplaceAll(err.Error(), "\n", " "))
	}
	return parsed.Verifiers, nil
}

// validateSpec returns "" when the spec is usable, else the reason.
func validateSpec(cfg *config.Config, s *Spec) string {
	if !specIDRe.MatchString(s.ID) {
		return "invalid id " + quote(s.ID) + ": use lowercase letters, digits, '.', '_' and '-'"
	}
	switch s.Severity {
	case "", severityError, severityWarning, "info":
	default:
		return "invalid severity " + quote(s.Severity) + ": use error, warning or info"
	}
	if msg := validateTarget(cfg, s); msg != "" {
		return msg
	}
	for _, g := range append(append([]string{}, s.WhenChanged...), s.Exclude...) {
		if _, err := vspec.CompileGlob(g); err != nil {
			return "invalid glob " + quote(g) + ": " + err.Error()
		}
	}
	if s.Require == nil {
		return "needs a [verifiers.require] predicate"
	}
	needs := predicateNeeds{}
	if msg := validateRequire(s.Require, 1, &needs); msg != "" {
		return msg
	}
	if msg := validateLLMPlacement(s.Require, true); msg != "" {
		return msg
	}
	if needs.scope && len(s.WhenChanged) == 0 {
		return "this predicate checks the changed files, so the verifier needs when_changed"
	}
	return validateExamples(s)
}

func quote(s string) string { return `"` + s + `"` }

type predicateNeeds struct{ scope, command, llm bool }

func validateTarget(cfg *config.Config, s *Spec) string {
	n := 0
	for _, v := range []string{s.Rule, s.Skill, s.Agent, s.Command} {
		if v != "" {
			n++
		}
	}
	if n != 1 {
		return "set exactly one of rule, skill, agent or command: the item this verifier enforces"
	}
	kind, id := s.TargetKind()
	if cfg.Content == nil {
		return ""
	}
	cf, ok := findTarget(cfg, kind, id)
	if !ok {
		return "the " + kind + " " + quote(id) + " does not exist (a verifier must name the item it enforces)"
	}
	if s.Anchor != "" && anchorLine(cf.Content, s.Anchor) == 0 {
		return "anchor " + quote(s.Anchor) + " is not a heading line of " + kind + " " + quote(id)
	}
	return ""
}

func validateRequire(r *Require, depth int, needs *predicateNeeds) string {
	if depth > maxDepth {
		return "all/any/not nest deeper than 4 levels"
	}
	set := 0
	for _, b := range []bool{r.Regex != nil, r.Forbid != nil, r.FileExists != nil, r.Paired != nil,
		r.GlobCount != nil, r.Command != nil, r.LLM != nil, len(r.All) > 0, len(r.Any) > 0, r.Not != nil} {
		if b {
			set++
		}
	}
	if set != 1 {
		return "a predicate table must set exactly one of regex, forbid, file_exists, paired, glob_count, command, llm, all, any, not"
	}
	switch {
	case r.Regex != nil:
		return validateRegexPred("regex", r.Regex, needs)
	case r.Forbid != nil:
		return validateRegexPred("forbid", r.Forbid, needs)
	case r.FileExists != nil:
		return validateFileExists(r.FileExists, needs)
	case r.Paired != nil:
		needs.scope = true
		return validatePaired(r.Paired)
	case r.GlobCount != nil:
		return validateGlobCount(r.GlobCount)
	case r.Command != nil:
		return validateCommand(r.Command, needs)
	case r.LLM != nil:
		return validateLLM(r.LLM, needs)
	case len(r.All) > 0:
		return validateChildren("all", r.All, depth, needs)
	case len(r.Any) > 0:
		return validateChildren("any", r.Any, depth, needs)
	}
	return validateRequire(r.Not, depth+1, needs)
}

func validateChildren(kind string, kids []Require, depth int, needs *predicateNeeds) string {
	for i := range kids {
		if msg := validateRequire(&kids[i], depth+1, needs); msg != "" {
			return kind + "[" + itoa(i) + "]: " + msg
		}
	}
	return ""
}

func validateRegexPred(kind string, p *RegexPred, needs *predicateNeeds) string {
	if p.Regex == "" {
		return kind + " needs a regex"
	}
	if _, err := regexp.Compile(p.Regex); err != nil {
		return kind + ": invalid regex: " + err.Error()
	}
	switch p.In {
	case "", inSameFile, inDiffAdded:
		needs.scope = true
		if p.Files != "" {
			return kind + ": files applies to in = \"any-file\" only"
		}
	case inAnyFile:
		if p.Files == "" {
			needs.scope = true
		} else if _, err := vspec.CompileGlob(p.Files); err != nil {
			return kind + ": invalid files glob: " + err.Error()
		}
	default:
		return kind + ": invalid in " + quote(p.In) + ": use same-file, diff-added or any-file"
	}
	return ""
}

func validateFileExists(p *FileExistsPred, needs *predicateNeeds) string {
	if p.Path == "" {
		return "file_exists needs a path"
	}
	if hasTemplate(p.Path) {
		needs.scope = true
		return validateTemplate("file_exists.path", p.Path)
	}
	if clean := path.Clean(strings.ReplaceAll(p.Path, "\\", "/")); path.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") {
		return "file_exists.path must stay inside the project"
	}
	return ""
}

func validatePaired(p *PairedPred) string {
	if p.ForEach == "" {
		return "paired needs for_each"
	}
	if (p.RequiresChanged == "") == (p.RequiresExists == "") {
		return "paired needs exactly one of requires_changed or requires_exists"
	}
	if strings.Contains(p.ForEach, "{rel}") {
		if strings.ContainsAny(strings.ReplaceAll(p.ForEach, "{rel}", ""), "*?[]{}") {
			return "paired.for_each with {rel} must otherwise be a literal path"
		}
	} else if _, err := vspec.CompileGlob(p.ForEach); err != nil {
		return "paired.for_each: " + err.Error()
	}
	return validateTemplate("paired.requires", p.RequiresChanged+p.RequiresExists)
}

func validateGlobCount(p *GlobCountPred) string {
	if p.Files == "" {
		return "glob_count needs files"
	}
	if p.Min == nil && p.Max == nil {
		return "glob_count needs min or max"
	}
	if (p.Min != nil && *p.Min < 0) || (p.Max != nil && *p.Max < 0) || (p.Min != nil && p.Max != nil && *p.Min > *p.Max) {
		return "glob_count min and max must be non-negative with min <= max"
	}
	for _, g := range append([]string{p.Files}, p.Exclude...) {
		if _, err := vspec.CompileGlob(g); err != nil {
			return "glob_count: invalid glob " + quote(g) + ": " + err.Error()
		}
	}
	return ""
}

func validateCommand(p *CommandPred, needs *predicateNeeds) string {
	if len(p.Argv) == 0 || strings.TrimSpace(p.Argv[0]) == "" {
		return "command needs a non-empty argv (the program and its arguments; there is no shell)"
	}
	for _, a := range p.Argv {
		if strings.ContainsRune(a, 0) {
			return "command.argv must not contain a NUL byte"
		}
	}
	switch p.PassFiles {
	case "":
	case passFilesArgs, passFilesStdin0:
		needs.scope = true
	default:
		return "command.pass_files " + quote(p.PassFiles) + " must be args or stdin0"
	}
	if p.TimeoutS < 0 || p.TimeoutS > maxCommandTimeoutS {
		return "command.timeout_s must be between 1 and " + itoa(maxCommandTimeoutS)
	}
	if p.ExpectExit != nil && (*p.ExpectExit < 0 || *p.ExpectExit > maxStatus) {
		return "command.expect_exit must be between 0 and 255"
	}
	needs.command = true
	return ""
}

func validateLLM(p *LLMPred, needs *predicateNeeds) string {
	if len(p.Checklist) == 0 || len(p.Checklist) > maxChecklist {
		return "llm.checklist needs 1 to " + itoa(maxChecklist) + " items"
	}
	for i, item := range p.Checklist {
		if strings.TrimSpace(item) == "" || len(item) > maxChecklistItem {
			return "llm.checklist[" + itoa(i) + "] must be a statement of at most " + itoa(maxChecklistItem) + " bytes"
		}
	}
	if p.MaxDiffBytes != 0 && (p.MaxDiffBytes < minMaxDiffBytes || p.MaxDiffBytes > maxMaxDiffBytes) {
		return "llm.max_diff_bytes must be between " + itoa(minMaxDiffBytes) + " and " + itoa(maxMaxDiffBytes)
	}
	if strings.ContainsFunc(p.Model, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		return "llm.model must not contain control characters"
	}
	needs.scope = true
	needs.llm = true
	return ""
}

// validateLLMPlacement allows an llm predicate only at the root of the tree or
// directly under `all`: under `any` or `not` its advisory verdict would decide
// the verifier, and a deterministic failure must always win.
func validateLLMPlacement(r *Require, allowed bool) string {
	if r.LLM != nil && !allowed {
		return "the llm predicate may only be the root predicate or a direct member of `all`"
	}
	for i := range r.All {
		if msg := validateLLMPlacement(&r.All[i], allowed); msg != "" {
			return msg
		}
	}
	for i := range r.Any {
		if msg := validateLLMPlacement(&r.Any[i], false); msg != "" {
			return msg
		}
	}
	if r.Not != nil {
		return validateLLMPlacement(r.Not, false)
	}
	return ""
}

func validateExamples(s *Spec) string {
	for i, ex := range s.Examples {
		switch ex.Expect {
		case "pass", "fail", "not_applicable":
		default:
			return "examples[" + itoa(i) + "].expect must be pass, fail or not_applicable"
		}
		for name, content := range ex.Files {
			if _, err := cleanRel(name); err != nil || name == "" {
				return "examples[" + itoa(i) + "] has a file path outside the fixture: " + quote(name)
			}
			if len(content) > maxExampleBytes {
				return "examples[" + itoa(i) + "] file " + quote(name) + " is too large"
			}
		}
	}
	return ""
}

// findTarget locates the content file a verifier names: `id` among root and
// domain items, or `domain/id` in that domain.
func findTarget(cfg *config.Config, kind, id string) (config.ContentFile, bool) {
	return findTargetIn(cfg.Content, kind, id)
}

// findTargetIn is findTarget over an explicit content tree.
func findTargetIn(content *config.ContentTree, kind, id string) (config.ContentFile, bool) {
	pick := func(files []config.ContentFile, name string) (config.ContentFile, bool) {
		for _, f := range files {
			if f.Name == name {
				return f, true
			}
		}
		return config.ContentFile{}, false
	}
	listOf := func(root *config.ContentTree, d *config.Domain) []config.ContentFile {
		switch kind {
		case "rule":
			if d != nil {
				return d.Rules
			}
			return root.Rules
		case "skill":
			if d != nil {
				return d.Skills
			}
			return root.Skills
		case "agent":
			if d != nil {
				return d.Agents
			}
			return root.Agents
		default:
			if d != nil {
				return d.Commands
			}
			return root.Commands
		}
	}
	if domain, name, ok := strings.Cut(id, "/"); ok {
		d := content.Domains[domain]
		if d == nil {
			return config.ContentFile{}, false
		}
		return pick(listOf(content, d), name)
	}
	if f, ok := pick(listOf(content, nil), id); ok {
		return f, true
	}
	names := make([]string, 0, len(content.Domains))
	for n := range content.Domains {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		if f, ok := pick(listOf(content, content.Domains[n]), id); ok {
			return f, true
		}
	}
	return config.ContentFile{}, false
}

// anchorLine returns the 1-based line of the heading equal to anchor, or 0.
func anchorLine(content, anchor string) int {
	want := strings.TrimSpace(anchor)
	for i, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(strings.TrimSuffix(line, "\r")) == want {
			return i + 1
		}
	}
	return 0
}

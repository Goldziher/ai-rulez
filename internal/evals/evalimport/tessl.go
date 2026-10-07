package evalimport

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Goldziher/ai-rulez/v5/internal/evals"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
)

// SourceTessl is the --from value of the Tessl scenario importer.
const SourceTessl = "tessl"

// Files of a scenario directory.
const (
	criteriaFile = "criteria.json"
	taskFile     = "task.md"
)

// defaultPassMark is the rubric pass mark the eval format applies when a case
// names none; an input without a threshold gets it, with a note.
const defaultPassMark = evals.DefaultRubricMinScore

// Tessl imports scenarios written as a task (task.md) plus a weighted checklist
// (criteria.json). The shape of criteria.json is not specified anywhere this
// importer could check, so the mapping is tolerant by design: a few spellings of
// each field are accepted, and every field that is not consumed is reported as
// unmapped instead of guessed at. The documented shape is
//
//	{"scenario": "add-health-endpoint",
//	 "criteria": [{"name": "adds route", "description": "Registers GET /health", "weight": 3}],
//	 "pass_threshold": 0.7}
//
// and is illustrative: it comes from public notes, not from a schema or a sample
// of the service's real files.
type Tessl struct{}

// Name implements Source.
func (Tessl) Name() string { return SourceTessl }

// Detect implements Source: a criteria file, or a directory that holds one or
// whose subdirectories do.
func (Tessl) Detect(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	if !info.IsDir() {
		return filepath.Base(path) == criteriaFile
	}
	if fileExists(filepath.Join(path, criteriaFile)) {
		return true
	}
	return len(scenarioDirs(path)) > 0
}

func fileExists(p string) bool {
	info, err := os.Lstat(p)
	return err == nil && info.Mode().IsRegular()
}

// scenarioDirs lists the subdirectories of dir that hold a criteria file.
func scenarioDirs(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() && fileExists(filepath.Join(dir, e.Name(), criteriaFile)) {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(out)
	return out
}

// Load implements Source.
func (t Tessl) Load(path string, limits Limits) ([]Scenario, error) {
	limits = limits.withDefaults()
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var dirs []string
	switch {
	case !info.IsDir():
		if filepath.Base(path) != criteriaFile {
			return nil, fmt.Errorf("%s: a scenario file must be named %s", path, criteriaFile)
		}
		dirs = []string{filepath.Dir(path)}
	case fileExists(filepath.Join(path, criteriaFile)):
		dirs = []string{path}
	default:
		dirs = scenarioDirs(path)
		if len(dirs) == 0 {
			return nil, fmt.Errorf("%s holds no %s, directly or in a subdirectory", path, criteriaFile)
		}
	}
	if len(dirs) > limits.MaxScenarios {
		return nil, fmt.Errorf("%s holds %d scenarios; the limit is %d", path, len(dirs), limits.MaxScenarios)
	}
	out := make([]Scenario, 0, len(dirs))
	for _, dir := range dirs {
		sc, err := loadScenario(dir, path, limits)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", dir, err)
		}
		out = append(out, sc)
	}
	return out, nil
}

// loadScenario reads one scenario directory.
func loadScenario(dir, origin string, limits Limits) (Scenario, error) {
	sc := Scenario{Dir: dir, Origin: origin}
	criteria, err := readRegular(filepath.Join(dir, criteriaFile), limits.MaxBytes)
	if err != nil {
		return sc, err
	}
	if !utf8.Valid(criteria) {
		return sc, fmt.Errorf("%s is not valid UTF-8", criteriaFile)
	}
	var raw any
	if err := json.Unmarshal(criteria, &raw); err != nil {
		return sc, fmt.Errorf("%s is not valid JSON: %w", criteriaFile, err)
	}
	if depth(raw) > limits.MaxDepth {
		return sc, fmt.Errorf("%s is nested deeper than %d levels", criteriaFile, limits.MaxDepth)
	}
	if _, ok := raw.(map[string]any); !ok {
		return sc, fmt.Errorf("%s must hold a JSON object", criteriaFile)
	}
	sc.Raw = raw
	sum := sha256.New()
	sum.Write(criteria)
	if _, statErr := os.Lstat(filepath.Join(dir, taskFile)); statErr == nil {
		task, err := readRegular(filepath.Join(dir, taskFile), limits.MaxBytes)
		if err != nil {
			return sc, err
		}
		if !utf8.Valid(task) {
			return sc, fmt.Errorf("%s is not valid UTF-8", taskFile)
		}
		sc.Task = string(task)
		sum.Write(task)
	}
	sc.SHA256 = "sha256:" + hex.EncodeToString(sum.Sum(nil))
	return sc, nil
}

// readRegular reads a regular file (not a symlink, not a device) of at most max bytes.
func readRegular(path string, limit int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err //nolint:wrapcheck // the message names the path
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file (symlinks are refused)", filepath.Base(path))
	}
	if info.Size() > limit {
		return nil, fmt.Errorf("%s is %d bytes; the limit is %d", filepath.Base(path), info.Size(), limit)
	}
	f, err := os.Open(path) //nolint:gosec // a path the caller named, checked above
	if err != nil {
		return nil, err //nolint:wrapcheck // the message names the path
	}
	defer f.Close() //nolint:errcheck // read-only
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err //nolint:wrapcheck // the message names the path
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s is larger than %d bytes", filepath.Base(path), limit)
	}
	return data, nil
}

// depth is the nesting depth of a decoded JSON value.
func depth(v any) int {
	switch t := v.(type) {
	case map[string]any:
		d := 0
		for _, c := range t {
			d = max(d, depth(c))
		}
		return d + 1
	case []any:
		d := 0
		for _, c := range t {
			d = max(d, depth(c))
		}
		return d + 1
	}
	return 0
}

// The accepted spellings of each field, most specific first.
var (
	nameKeys      = []string{"scenario", "name", "id", "title"}
	taskKeys      = []string{"task", "prompt"}
	criteriaKeys  = []string{"criteria", "checklist", "rubric"}
	thresholdKeys = []string{"pass_threshold", "passing_score", "pass_score", "passing_threshold", "threshold"}
	fixtureKeys   = []string{"files", "fixtures", "starting_files", "setup_files"}
	activationKey = []string{"activation", "activation_test", "activation_tests"}
	critNameKeys  = []string{"name", "title", "id"}
	critTextKeys  = []string{"description", "criterion", "text", "check", "prompt"}
	critWeightKey = []string{"weight", "points", "score", "max_score"}
	negativeKeys  = []string{"should_not_trigger", "near_miss", "negative", "negatives", "negative_prompts"}
)

// hints says where an ignored field belongs in ai-rulez.
var hints = map[string]string{
	"baseline": "use `eval run --ablation`",
	"repeats":  "use `eval run --runs N`",
	"runs":     "use `eval run --runs N`",
	"agent":    "use `eval run --harness` and `--model`",
	"model":    "use `eval run --model` (or a case's `model`)",
	"harness":  "use `eval run --harness`",
}

// consumed tracks which JSON paths were mapped, so the rest can be reported.
type consumed map[string]bool

func (c consumed) mark(path string) { c[path] = true }

// typed returns v as T, or T's zero value when v holds another type.
func typed[T any](v any) T {
	if t, ok := v.(T); ok {
		return t
	}
	var zero T
	return zero
}

// Map implements Source.
func (t Tessl) Map(sc *Scenario, opts MapOptions) (*Mapped, error) {
	doc := typed[map[string]any](sc.Raw)
	used := consumed{}
	m := &Mapped{Report: Report{Source: sc.Origin, SourceSHA256: sc.SHA256}}
	rep := &m.Report

	name := firstString(doc, nameKeys, used)
	if name == "" {
		name = filepath.Base(sc.Dir)
		rep.Assumed = append(rep.Assumed, fmt.Sprintf("scenario name %q taken from the directory name", name))
	}
	m.Report.Scenario = name
	m.ID = Slug(name)
	if opts.ID != "" {
		m.ID = opts.ID
	}

	task := sc.Task
	if strings.TrimSpace(task) == "" {
		task = firstString(doc, taskKeys, used)
	}
	if strings.TrimSpace(task) == "" {
		return nil, fmt.Errorf("scenario %q has no task: add a %s next to %s", name, taskFile, criteriaFile)
	}
	m.TaskFile, m.TaskContent = m.ID+".task.md", task
	rep.Mapped = append(rep.Mapped, "task -> prompt_file")

	criteria, err := readCriteria(doc, used)
	if err != nil {
		return nil, fmt.Errorf("scenario %q: %w", name, err)
	}
	if err := t.mapRubric(m, criteria, doc, used, opts); err != nil {
		return nil, fmt.Errorf("scenario %q: %w", name, err)
	}
	if err := mapFixtures(m, sc, doc, used); err != nil {
		return nil, fmt.Errorf("scenario %q: %w", name, err)
	}
	mapActivation(m, doc, used)

	m.Case.ID = m.ID
	m.Case.Description = "Imported scenario"
	m.Case.PromptFile = m.TaskFile
	yes := true
	m.Case.ExpectTrigger = &yes
	m.Case.Tags = []string{"imported:" + SourceTessl}
	rep.Assumed = append(rep.Assumed, "expect_trigger: true (the scenario targets a skill)")

	if opts.LiftAssertions {
		lifted := liftAssertions(criteria)
		text := false
		for _, l := range lifted {
			m.Case.Assertions = append(m.Case.Assertions, l.assertion)
			rep.Lifted = append(rep.Lifted, Lift{Criterion: l.criterion, Assertion: describeAssertion(l.assertion)})
			text = text || l.assertion.Type == evals.AssertContains || l.assertion.Type == evals.AssertNotContains
		}
		if text {
			// The criterion's wording is matched case-insensitively, the assertion it
			// becomes is not: say so, since a not_contains misses "Draft" for "DRAFT".
			rep.Assumed = append(rep.Assumed, "lifted contains and not_contains assertions match case-sensitively; the criteria they came from do not say so")
		}
	}
	m.Report.Unmapped = unmappedFields(doc, used)
	sort.Slice(m.Report.Unmapped, func(a, b int) bool { return m.Report.Unmapped[a].Path < m.Report.Unmapped[b].Path })
	return m, scanScenario(m, criteria)
}

// criterion is one checklist entry after reading.
type criterion struct {
	name   string
	text   string
	weight float64
	// explicit says the input gave a weight (not the default of 1).
	explicit bool
	path     string
}

// label is how a criterion is named in messages: its name, else its text.
func (c criterion) label() string {
	if c.name != "" {
		return c.name
	}
	return c.text
}

// body is the criterion as a grader reads it: the description, else the name.
func (c criterion) body() string {
	if strings.TrimSpace(c.text) != "" {
		return strings.TrimSpace(c.text)
	}
	return strings.TrimSpace(c.name)
}

// readCriteria reads the checklist: a list of objects or strings, or an object of
// name to description-or-object. Zero-weight entries are dropped (and later
// reported); a negative weight, or no weight at all across the list, is an error.
func readCriteria(doc map[string]any, used consumed) ([]criterion, error) {
	key, raw := firstKey(doc, criteriaKeys)
	if key == "" {
		return nil, fmt.Errorf("there is no checklist: expected one of %s", strings.Join(criteriaKeys, ", "))
	}
	used.mark("$." + key)
	var out []criterion
	switch t := raw.(type) {
	case []any:
		for i, entry := range t {
			c, err := readCriterion(entry, fmt.Sprintf("$.%s[%d]", key, i), used)
			if err != nil {
				return nil, err
			}
			out = append(out, c)
		}
	case map[string]any:
		for _, k := range sortedKeys(t) {
			path := child("$."+key, k)
			c, err := readCriterion(t[k], path, used)
			if err != nil {
				return nil, err
			}
			if c.name == "" {
				c.name = k
			}
			out = append(out, c)
		}
	default:
		return nil, fmt.Errorf("%s must be a list or an object", key)
	}
	var kept []criterion
	total := 0.0
	for _, c := range out {
		switch {
		case c.weight < 0:
			return nil, fmt.Errorf("criterion %q has a negative weight (%v)", c.label(), c.weight)
		case c.weight == 0:
			continue
		}
		total += c.weight
		kept = append(kept, c)
	}
	if len(kept) == 0 || total <= 0 {
		return nil, fmt.Errorf("the checklist has no criterion with a positive weight")
	}
	return kept, nil
}

func readCriterion(entry any, path string, used consumed) (criterion, error) {
	c := criterion{weight: 1, path: path}
	switch t := entry.(type) {
	case string:
		c.text = t
		used.mark(path)
	case map[string]any:
		used.mark(path)
		if k, v := firstKey(t, critNameKeys); k != "" {
			c.name = typed[string](v)
			used.mark(path + "." + k)
		}
		if k, v := firstKey(t, critTextKeys); k != "" {
			c.text = typed[string](v)
			used.mark(path + "." + k)
		}
		if k, v := firstKey(t, critWeightKey); k != "" {
			w, ok := number(v)
			if !ok || math.IsNaN(w) || math.IsInf(w, 0) {
				return c, fmt.Errorf("criterion %q has a weight that is not a finite number", c.label())
			}
			c.weight, c.explicit = w, true
			used.mark(path + "." + k)
		}
	default:
		return c, fmt.Errorf("%s is neither a string nor an object", path)
	}
	if strings.TrimSpace(c.body()) == "" {
		return c, fmt.Errorf("%s has no description", path)
	}
	return c, nil
}

// mapRubric builds the rubric (or rubric_items) and the pass mark.
func (Tessl) mapRubric(m *Mapped, criteria []criterion, doc map[string]any, used consumed, opts MapOptions) error {
	rep := &m.Report
	total := 0.0
	for _, c := range criteria {
		total += c.weight
	}
	explicit := false
	for _, c := range criteria {
		explicit = explicit || c.explicit
	}
	if !explicit {
		rep.Assumed = append(rep.Assumed, "every criterion weighs 1 (the input gives no weights)")
	}
	switch opts.RubricMode {
	case RubricItems:
		for _, c := range criteria {
			w := c.weight
			m.Case.RubricItems = append(m.Case.RubricItems, evals.RubricItem{Text: c.body(), Weight: &w})
		}
		rep.Mapped = append(rep.Mapped, fmt.Sprintf("%d criteria -> rubric_items (weights kept)", len(criteria)))
	default:
		var b strings.Builder
		fmt.Fprintf(&b, "Score the answer against this weighted checklist (total weight %s):\n", formatNumber(total))
		for i, c := range criteria {
			fmt.Fprintf(&b, "%d. (weight %s) %s\n", i+1, formatNumber(c.weight), c.body())
		}
		m.Case.Rubric = strings.TrimRight(b.String(), "\n")
		rep.Mapped = append(rep.Mapped, fmt.Sprintf("%d criteria -> rubric (weights kept as text)", len(criteria)))
	}
	key, raw := firstKey(doc, thresholdKeys)
	if key == "" {
		rep.Assumed = append(rep.Assumed, fmt.Sprintf("no pass threshold; the eval default %.2f applies", defaultPassMark))
		return nil
	}
	used.mark("$." + key)
	value, unit, err := parseThreshold(raw)
	if err != nil {
		return fmt.Errorf("%s: %w", key, err)
	}
	m.Case.RubricMinScore = &value
	rep.Mapped = append(rep.Mapped, fmt.Sprintf("%s -> rubric_min_score %s (read as %s)", key, formatNumber(value), unit))
	return nil
}

var percentText = regexp.MustCompile(`^\s*(\d+(?:\.\d+)?)\s*%\s*$`)

// parseThreshold reads a pass mark as a fraction (0-1) or a percent (above 1, up
// to 100, or written with a percent sign) and returns it as a fraction.
func parseThreshold(raw any) (value float64, unit string, err error) {
	if s, ok := raw.(string); ok {
		if m := percentText.FindStringSubmatch(s); m != nil {
			v, _ := strconv.ParseFloat(m[1], 64) //nolint:errcheck // the pattern guarantees a number
			return percentToFraction(v)
		}
		v, perr := strconv.ParseFloat(strings.TrimSpace(s), 64)
		if perr != nil {
			return 0, "", fmt.Errorf("%q is not a number", summarize(s))
		}
		raw = v
	}
	v, ok := number(raw)
	if !ok || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return 0, "", fmt.Errorf("must be a number between 0 and 100")
	}
	if v <= 1 {
		return round4(v), "a fraction", nil
	}
	return percentToFraction(v)
}

func percentToFraction(v float64) (fraction float64, unit string, err error) {
	if v > 100 {
		return 0, "", fmt.Errorf("%v is above 100", v)
	}
	return round4(v / 100), "a percent", nil
}

func round4(v float64) float64 { return math.Round(v*1e4) / 1e4 }

// mapFixtures maps starting files.
func mapFixtures(m *Mapped, sc *Scenario, doc map[string]any, used consumed) error {
	key, raw := firstKey(doc, fixtureKeys)
	if key == "" {
		return nil
	}
	used.mark("$." + key)
	var files []evals.Fixture
	add := func(path, content, source string) error {
		if msg := evals.CheckRelPath(path); msg != "" {
			return fmt.Errorf("fixture path %s", msg)
		}
		if source != "" {
			if msg := evals.CheckRelPath(source); msg != "" {
				return fmt.Errorf("fixture source %s", msg)
			}
			data, err := readInside(sc.Dir, source)
			if err != nil {
				return fmt.Errorf("fixture %q: %w", path, err)
			}
			dest := "fixtures/" + m.ID + "/" + filepath.ToSlash(filepath.Clean(path))
			m.Fixtures = append(m.Fixtures, FixtureFile{Path: dest, Content: data})
			files = append(files, evals.Fixture{Path: path, Source: dest})
			return nil
		}
		files = append(files, evals.Fixture{Path: path, Content: content})
		return nil
	}
	var err error
	switch t := raw.(type) {
	case []any:
		err = mapFixtureList(key, t, add, used)
	case map[string]any:
		err = mapFixtureObject(key, t, add, used)
	default:
		err = fmt.Errorf("$.%s must be a list or an object", key)
	}
	if err != nil {
		return err
	}
	m.Case.Files = files
	m.Report.Mapped = append(m.Report.Mapped, fmt.Sprintf("%d fixture(s) -> files", len(files)))
	return nil
}

// fixtureAdder records one fixture file for the mapped case.
type fixtureAdder func(path, content, source string) error

// mapFixtureList maps the list form of the fixtures: objects with a path and a content or source.
func mapFixtureList(key string, list []any, add fixtureAdder, used consumed) error {
	for i, entry := range list {
		obj, ok := entry.(map[string]any)
		if !ok {
			return fmt.Errorf("$.%s[%d] must be an object with a path", key, i)
		}
		p := typed[string](obj["path"])
		content := typed[string](obj["content"])
		source := typed[string](obj["source"])
		if content != "" && source != "" {
			return fmt.Errorf("$.%s[%d] has both content and source", key, i)
		}
		if err := add(p, content, source); err != nil {
			return err
		}
		for _, k := range []string{"path", "content", "source"} {
			used.mark(fmt.Sprintf("$.%s[%d].%s", key, i, k))
		}
		used.mark(fmt.Sprintf("$.%s[%d]", key, i))
	}
	return nil
}

// mapFixtureObject maps the object form of the fixtures: file path to content.
func mapFixtureObject(key string, obj map[string]any, add fixtureAdder, used consumed) error {
	for _, p := range sortedKeys(obj) {
		content, ok := obj[p].(string)
		if !ok {
			return fmt.Errorf("%s must be the file's content as a string", child("$."+key, p))
		}
		if err := add(p, content, ""); err != nil {
			return err
		}
		used.mark(child("$."+key, p))
	}
	return nil
}

// readInside reads a file under base, refusing anything that resolves outside it.
func readInside(base, rel string) ([]byte, error) {
	full := filepath.Join(base, filepath.FromSlash(rel))
	resolvedBase, err := filepath.EvalSymlinks(base)
	if err != nil {
		return nil, err //nolint:wrapcheck // the message names the path
	}
	resolved, err := filepath.EvalSymlinks(full)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s", rel)
	}
	if r, err := filepath.Rel(resolvedBase, resolved); err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("%s resolves outside the scenario directory", rel)
	}
	return readRegular(resolved, DefaultMaxBytes)
}

// mapActivation maps an activation-test block: prompts that must not fire become
// near misses.
func mapActivation(m *Mapped, doc map[string]any, used consumed) {
	key, raw := firstKey(doc, activationKey)
	if key == "" {
		return
	}
	used.mark("$." + key)
	obj, ok := raw.(map[string]any)
	if !ok {
		return
	}
	nk, nraw := firstKey(obj, negativeKeys)
	if nk == "" {
		return
	}
	list := typed[[]any](nraw)
	used.mark(fmt.Sprintf("$.%s.%s", key, nk))
	for _, e := range list {
		if s, ok := e.(string); ok && strings.TrimSpace(s) != "" {
			m.Case.NearMiss = append(m.Case.NearMiss, strings.TrimSpace(s))
		}
	}
	if len(m.Case.NearMiss) > 0 {
		m.Report.Mapped = append(m.Report.Mapped, fmt.Sprintf("activation block -> %d near_miss prompt(s)", len(m.Case.NearMiss)))
	}
}

// unmappedFields walks the document and lists every leaf path (or subtree) no
// mapping consumed.
func unmappedFields(doc map[string]any, used consumed) []Unmapped {
	var out []Unmapped
	var walk func(path string, v any)
	walk = func(path string, v any) {
		if used[path] {
			// a consumed container may still hold fields nobody read
			switch t := v.(type) {
			case map[string]any:
				for _, k := range sortedKeys(t) {
					walk(child(path, k), t[k])
				}
			case []any:
				for i, e := range t {
					walk(fmt.Sprintf("%s[%d]", path, i), e)
				}
			}
			return
		}
		out = append(out, Unmapped{Path: path, Value: summarize(v), Hint: hints[lastKey(path)]})
	}
	for _, k := range sortedKeys(doc) {
		walk(child("$", k), doc[k])
	}
	return out
}

// simpleKey is a JSON key that can stand in a path as written.
var simpleKey = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// child extends a JSON path with an object key. A key that is not plain is written
// as a quoted index, so a hostile key (control characters, escape sequences) can
// neither forge another path nor reach a terminal raw.
func child(path, key string) string {
	if simpleKey.MatchString(key) {
		return path + "." + key
	}
	return path + "[" + strconv.Quote(summarize(key)) + "]"
}

func lastKey(path string) string {
	if i := strings.LastIndex(path, "."); i >= 0 && !strings.ContainsAny(path[i:], "[]\"") {
		return path[i+1:]
	}
	return ""
}

// scanScenario refuses text that must not be imported: hidden characters (AR002)
// and credentials (AR001) anywhere, and flags instruction-override phrases
// (AR004), which end up in a rubric a grader model reads.
func scanScenario(m *Mapped, criteria []criterion) error {
	check := func(where, text string) error {
		for _, f := range lint.ScanText("scenario", text) {
			switch f.Code {
			case lint.CodeHiddenCharacters, lint.CodeSecretDetected:
				return fmt.Errorf("refusing scenario %q: %s line %d: %s (%s)", m.Report.Scenario, where, f.Line, f.Message, f.Code)
			case lint.CodeInjectionPhrase:
				m.Report.Warnings = append(m.Report.Warnings, fmt.Sprintf("%s line %d: %s (%s)", where, f.Line, f.Message, f.Code))
			}
		}
		return nil
	}
	if err := check("the task", m.TaskContent); err != nil {
		return err
	}
	for _, c := range criteria {
		if err := check("criterion "+strconv.Quote(c.label()), c.name+"\n"+c.text); err != nil {
			return err
		}
	}
	for _, f := range m.Fixtures {
		if err := check("fixture "+f.Path, string(f.Content)); err != nil {
			return err
		}
	}
	for _, f := range m.Case.Files {
		if err := check("fixture "+f.Path, f.Content); err != nil {
			return err
		}
	}
	for _, p := range m.Case.NearMiss {
		if err := check("a near-miss prompt", p); err != nil {
			return err
		}
	}
	return nil
}

// Slug turns a scenario name into a case id: lowercase letters, digits, '.', '_'
// and '-', starting with a letter or digit, at most 64 characters.
func Slug(name string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(name) {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_'
		if ok {
			b.WriteRune(r)
			prevDash = false
			continue
		}
		if !prevDash {
			b.WriteByte('-')
			prevDash = true
		}
	}
	s := strings.Trim(b.String(), "-._")
	if len(s) > 64 {
		s = strings.Trim(s[:64], "-._")
	}
	if s == "" {
		return "scenario"
	}
	return s
}

// firstKey returns the first of keys present in obj.
func firstKey(obj map[string]any, keys []string) (key string, value any) {
	for _, k := range keys {
		if v, ok := obj[k]; ok {
			return k, v
		}
	}
	return "", nil
}

func firstString(obj map[string]any, keys []string, used consumed) string {
	for _, k := range keys {
		if s, ok := obj[k].(string); ok && strings.TrimSpace(s) != "" {
			used.mark("$." + k)
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func number(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case json.Number:
		f, err := t.Float64()
		return f, err == nil
	}
	return 0, false
}

func formatNumber(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

var _ Source = Tessl{}

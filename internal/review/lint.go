package review

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/samber/oops"
	"gopkg.in/yaml.v3"

	"github.com/Goldziher/ai-rulez/v5/internal/lint"
)

// Problem is one defect of a rubric, golden or calibration file (AR9G8).
type Problem struct {
	File    string
	Line    int
	Message string
}

// maxSystemPromptBytes bounds system.md so a rubric cannot smuggle an unreviewable prompt.
const maxSystemPromptBytes = 64 << 10

// weightTolerance is how far the weights may sit from 1 (float rounding of decimal weights).
const weightTolerance = 0.001

// goldenFile is one golden/*.golden.yaml case.
//
//nolint:tagliatelle // golden keys are snake_case by project convention
type goldenFile struct {
	SchemaVersion int `yaml:"schema_version"`
	ID            string
	Kind          string
	Item          struct{ Path string }
	Siblings      []string
	Labelers      []struct {
		ID     string
		Labels map[string]string
	}
	Adjudicated map[string]string
	Probes      []string
}

var goldenProbes = map[string]bool{"pad": true, "reorder": true, "rename": true, "canary": true}

// calibrationFile is the part of calibration.json that lint checks.
//
//nolint:tagliatelle // calibration keys are snake_case by project convention
type calibrationFile struct {
	SchemaVersion int `json:"schema_version"`
	Rubric        struct {
		ID string `json:"id"`
	} `json:"rubric"`
	Status string `json:"status"`
}

// registeredCodes is the set of lint rule codes a twin may name.
func registeredCodes() map[string]bool {
	set := map[string]bool{}
	for _, r := range lint.Rules() {
		set[r.Code] = true
	}
	return set
}

// LintDir checks the rubric directory dir and returns every problem found.
// It never returns a partial rubric as valid: a directory with a problem has no
// usable rubric.
func LintDir(dir string) ([]Problem, error) {
	_, problems, err := loadDir(dir)
	return problems, err
}

// LintBuiltin checks an embedded rubric. A built-in rubric that fails is a bug
// of the binary, and a test keeps that from shipping.
func LintBuiltin(id string) ([]Problem, error) {
	r, err := LoadBuiltin(id)
	if err != nil {
		return nil, err
	}
	return lintRubric(r, filepath.ToSlash(filepath.Join("builtin", id, RubricFile)), nil), nil
}

// loadDir parses and lints one rubric directory. The returned rubric is nil
// when rubric.toml is missing or does not parse.
func loadDir(dir string) (*Rubric, []Problem, error) {
	file := filepath.Join(dir, RubricFile)
	info, err := os.Lstat(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("rubric directory %s: %w", dir, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, []Problem{{File: dir, Line: 1, Message: "the rubric directory must be a regular directory, not a symlink"}}, nil
	}
	data, rerr := readRegular(file)
	if rerr != nil {
		return nil, []Problem{{File: file, Line: 1, Message: "rubric.toml is missing, unreadable or a symlink: " + rerr.Error()}}, nil
	}
	r, unknown, perr := parseRubricLenient(data)
	if perr != nil {
		return nil, []Problem{{File: file, Line: tomlErrorLine(data), Message: perr.Error()}}, nil
	}
	r.Ref = filepath.Base(dir)
	r.Dir = dir
	files := []fileBytes{{RubricFile, data}}
	var problems []Problem
	for _, u := range unknown {
		problems = append(problems, Problem{File: file, Line: u.line, Message: "unknown key " + u.key})
	}

	if sys, serr := readRegular(filepath.Join(dir, SystemFile)); serr == nil {
		files = append(files, fileBytes{SystemFile, sys})
		if len(sys) > maxSystemPromptBytes {
			problems = append(problems, Problem{File: filepath.Join(dir, SystemFile), Line: 1, Message: fmt.Sprintf("system.md is %d bytes; the limit is %d", len(sys), maxSystemPromptBytes)})
		}
		r.SystemPrompt = string(sys)
	} else if _, statErr := os.Lstat(filepath.Join(dir, SystemFile)); statErr == nil {
		problems = append(problems, Problem{File: filepath.Join(dir, SystemFile), Line: 1, Message: "system.md is unreadable or a symlink"})
	}

	problems = append(problems, lintRubric(r, file, data)...)
	gp, gfiles := lintGolden(dir, r)
	problems = append(problems, gp...)
	files = append(files, gfiles...)
	cp, cfiles := lintCalibration(dir, r)
	problems = append(problems, cp...)
	files = append(files, cfiles...)
	r.Digest = digestOf(files)
	sortProblems(problems)
	return r, problems, nil
}

// unknownKey is a key of rubric.toml no field of a rubric takes.
type unknownKey struct {
	key  string
	line int
}

// parseRubricLenient is ParseRubric for lint: an unknown key does not stop the
// parse. The rubric decodes without it and every unknown key is returned, so one
// run reports them together with the other problems. Any other decode error
// (a syntax error, a wrong type) still returns no rubric.
func parseRubricLenient(data []byte) (*Rubric, []unknownKey, error) {
	var r Rubric
	dec := toml.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	err := dec.Decode(&r)
	if err == nil {
		return &r, nil, nil
	}
	var strict *toml.StrictMissingError
	if !errors.As(err, &strict) {
		return nil, nil, oops.Wrapf(err, "parse rubric")
	}
	var unknown []unknownKey
	for i := range strict.Errors {
		row, _ := strict.Errors[i].Position()
		unknown = append(unknown, unknownKey{key: strings.Join(strict.Errors[i].Key(), "."), line: max(row, 1)})
	}
	var lenient Rubric
	if err := toml.Unmarshal(data, &lenient); err != nil {
		return nil, nil, oops.Wrapf(err, "parse rubric")
	}
	return &lenient, unknown, nil
}

func sortProblems(p []Problem) {
	sort.SliceStable(p, func(i, j int) bool {
		if p[i].File != p[j].File {
			return p[i].File < p[j].File
		}
		return p[i].Line < p[j].Line
	})
}

// tomlErrorLine extracts the line of a TOML syntax error, or 1.
func tomlErrorLine(data []byte) int {
	var probe map[string]any
	err := toml.Unmarshal(data, &probe)
	var de *toml.DecodeError
	if errors.As(err, &de) {
		row, _ := de.Position()
		return max(row, 1)
	}
	return 1
}

// lineOf returns the 1-based line of the first line of raw containing needle, or 1.
func lineOf(raw []byte, needle string) int {
	if raw == nil || needle == "" {
		return 1
	}
	for i, l := range strings.Split(string(raw), "\n") {
		if strings.Contains(l, needle) {
			return i + 1
		}
	}
	return 1
}

// lintRubric checks the parsed content of rubric.toml. raw may be nil (a
// built-in rubric), in which case every problem points at line 1.
func lintRubric(r *Rubric, file string, raw []byte) []Problem {
	var out []Problem
	add := func(needle, format string, args ...any) {
		out = append(out, Problem{File: file, Line: lineOf(raw, needle), Message: fmt.Sprintf(format, args...)})
	}
	if r.SchemaVersion != SchemaVersion {
		add("schema_version", "schema_version must be %d, got %d", SchemaVersion, r.SchemaVersion)
	}
	if !idRe.MatchString(r.ID) {
		add("id =", "id %q must be lowercase letters, digits and single hyphens", r.ID)
	} else if !strings.HasPrefix(r.Ref, "builtin:") && r.Ref != "" && r.Ref != r.ID && r.Dir != "" {
		add("id =", "id %q must equal the directory name %q", r.ID, r.Ref)
	}
	if r.Version < 1 {
		add("version", "version must be an integer >= 1 (bump it on any change that can alter output)")
	}
	if len(r.AppliesTo) == 0 {
		add("applies_to", "applies_to must list at least one of skill, agent, command, rule")
	}
	for _, k := range r.AppliesTo {
		if !validKinds[k] {
			add("applies_to", "applies_to has unknown kind %q (use skill, agent, command, rule)", k)
		}
	}
	out = append(out, lintLimits(r, add)...)
	out = append(out, lintDimensions(r, file, raw)...)
	return out
}

func lintLimits(r *Rubric, add func(string, string, ...any)) []Problem {
	if r.Limits.MaxItemTokens < 0 || r.Limits.MaxSiblings < 0 || r.Limits.MaxOutputTokens < 0 {
		add("[limits]", "limits must not be negative")
	}
	lintVotes(r.Votes, add)
	lintCalibrationThresholds(r, add)
	return nil
}

func lintVotes(v Votes, add func(string, string, ...any)) {
	if v.Max < 0 || v.Max > 5 {
		add("[votes]", "votes.max must be between 0 and 5")
	}
	for name, t := range map[string]float64{"first_temperature": v.FirstTemperature, "extra_temperature": v.ExtraTemperature} {
		if t < 0 || t > 2 {
			add(name, "votes.%s must be between 0 and 2", name)
		}
	}
	if v.InstabilityThreshold < 0 || v.InstabilityThreshold > 1 {
		add("instability_threshold", "votes.instability_threshold must be between 0 and 1")
	}
}

func lintCalibrationThresholds(r *Rubric, add func(string, string, ...any)) {
	c := r.Calibration
	for name, t := range map[string]float64{"min_weighted_kappa": c.MinWeightedKappa, "min_consistency": c.MinConsistency, "min_human_kappa": c.MinHumanKappa} {
		if t < 0 || t > 1 {
			add(name, "calibration.%s must be between 0 and 1", name)
		}
	}
	if c.GoldenMinItems < 0 || c.MaxAgeDays < 0 {
		add("[calibration]", "calibration counts must not be negative")
	}
	ids := make([]string, 0, len(c.MinRecall))
	for id := range c.MinRecall {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if _, ok := r.Dimension(id); !ok {
			add(id, "calibration.min_recall names unknown dimension %q", id)
		}
		if t := c.MinRecall[id]; t < 0 || t > 1 {
			add(id, "calibration.min_recall.%s must be between 0 and 1", id)
		}
	}
}

func lintDimensions(r *Rubric, file string, raw []byte) []Problem {
	var out []Problem
	add := func(needle, format string, args ...any) {
		out = append(out, Problem{File: file, Line: lineOf(raw, needle), Message: fmt.Sprintf(format, args...)})
	}
	if len(r.Dimensions) == 0 {
		add("[[dimension]]", "a rubric needs at least one [[dimension]]")
		return out
	}
	codes := registeredCodes()
	seenID, seenCode := map[string]bool{}, map[string]bool{}
	sum := 0.0
	for _, d := range r.Dimensions {
		lintDimension(d, codes, seenID, seenCode, add)
		sum += d.Weight
	}
	if math.Abs(sum-1) > weightTolerance {
		add("weight", "dimension weights sum to %.3f; they must sum to 1", sum)
	}
	return out
}

// lintDimension checks one [[dimension]]; seenID and seenCode carry the earlier ones.
func lintDimension(d Dimension, codes, seenID, seenCode map[string]bool, add func(string, string, ...any)) {
	marker := `"` + d.ID + `"`
	if !idRe.MatchString(d.ID) {
		add("id =", "dimension id %q must be lowercase letters, digits and single hyphens", d.ID)
	}
	if seenID[d.ID] {
		add(marker, "duplicate dimension id %q", d.ID)
	}
	seenID[d.ID] = true
	if d.Code != "" && (!dimCodeRe.MatchString(d.Code) || !codes[d.Code]) {
		add(marker, "dimension %q code %q must be one of AR9G1-AR9G7 (the codes review registers)", d.ID, d.Code)
	}
	if d.Code != "" && seenCode[d.Code] {
		add(marker, "dimension %q reuses code %s", d.ID, d.Code)
	}
	if d.Code != "" {
		seenCode[d.Code] = true
	}
	if d.Group != GroupIntrinsic && d.Group != GroupContextual {
		add(marker, "dimension %q group must be %q or %q", d.ID, GroupIntrinsic, GroupContextual)
	}
	if math.IsNaN(d.Weight) || d.Weight <= 0 || d.Weight > 1 {
		add(marker, "dimension %q weight must be greater than 0 and at most 1", d.ID)
	}
	if d.Severity != "info" && d.Severity != "warning" {
		add(marker, "dimension %q severity must be \"info\" or \"warning\" (the judge never reports an error)", d.ID)
	}
	for _, f := range []struct{ name, text string }{{"question", d.Question}, {"pass", d.Pass}, {"warn", d.Warn}, {"fail", d.Fail}} {
		if strings.TrimSpace(f.text) == "" {
			add(marker, "dimension %q has an empty %s", d.ID, f.name)
		}
	}
	for _, t := range d.Twins {
		if !codes[t] || dimCodeRe.MatchString(t) {
			add(t, "dimension %q twin %q is not a registered lint rule code", d.ID, t)
		}
	}
}

func lintGolden(dir string, r *Rubric) ([]Problem, []fileBytes) {
	gdir := filepath.Join(dir, GoldenDir)
	entries, err := os.ReadDir(gdir)
	if err != nil {
		return nil, nil
	}
	var problems []Problem
	var files []fileBytes
	seen := map[string]string{}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".golden.yaml") {
			continue
		}
		path := filepath.Join(gdir, name)
		data, rerr := readRegular(path)
		if rerr != nil {
			problems = append(problems, Problem{File: path, Line: 1, Message: "golden file is unreadable or a symlink"})
			continue
		}
		files = append(files, fileBytes{GoldenDir + "/" + name, data})
		for _, m := range lintGoldenFile(data, r) {
			problems = append(problems, Problem{File: path, Line: 1, Message: m})
		}
		var g goldenFile
		if yaml.Unmarshal(data, &g) == nil && g.ID != "" {
			if prev, dup := seen[g.ID]; dup {
				problems = append(problems, Problem{File: path, Line: 1, Message: fmt.Sprintf("golden id %q is also used by %s", g.ID, prev)})
			}
			seen[g.ID] = name
		}
	}
	return problems, files
}

func lintGoldenFile(data []byte, r *Rubric) []string {
	var g goldenFile
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&g); err != nil {
		return []string{"golden file does not parse: " + err.Error()}
	}
	var out []string
	if g.SchemaVersion != SchemaVersion {
		out = append(out, fmt.Sprintf("schema_version must be %d", SchemaVersion))
	}
	if !idRe.MatchString(g.ID) {
		out = append(out, fmt.Sprintf("golden id %q must be lowercase letters, digits and single hyphens", g.ID))
	}
	if !validKinds[g.Kind] {
		out = append(out, fmt.Sprintf("golden kind %q must be skill, agent, command or rule", g.Kind))
	}
	for _, p := range append([]string{g.Item.Path}, g.Siblings...) {
		if !safeRelPath(p) {
			out = append(out, fmt.Sprintf("golden path %q must be a relative path without ..", p))
		}
	}
	if len(g.Labelers) < 2 {
		out = append(out, "a golden case needs at least two labelers")
	}
	for _, l := range g.Labelers {
		out = append(out, checkLabels(r, "labeler "+l.ID, l.Labels)...)
	}
	if len(g.Adjudicated) == 0 {
		out = append(out, "a golden case needs an adjudicated label")
	}
	out = append(out, checkLabels(r, "adjudicated", g.Adjudicated)...)
	for _, p := range g.Probes {
		if !goldenProbes[p] {
			out = append(out, fmt.Sprintf("unknown probe %q (use pad, reorder, rename, canary)", p))
		}
	}
	sort.Strings(out)
	return out
}

// checkLabels reports labels that name an unknown dimension or verdict.
func checkLabels(r *Rubric, who string, labels map[string]string) []string {
	var out []string
	for dim, v := range labels {
		if _, ok := r.Dimension(dim); !ok {
			out = append(out, fmt.Sprintf("%s labels unknown dimension %q", who, dim))
		}
		if _, ok := verdictValue[v]; !ok {
			out = append(out, fmt.Sprintf("%s gives %q verdict %q (use pass, warn, fail)", who, dim, v))
		}
	}
	return out
}

var driveRe = regexp.MustCompile(`^[A-Za-z]:`)

// safeRelPath reports whether p is a non-empty relative path with no .. element.
func safeRelPath(p string) bool {
	if p == "" || strings.HasPrefix(p, "/") || strings.HasPrefix(p, `\`) || driveRe.MatchString(p) {
		return false
	}
	for _, part := range strings.FieldsFunc(p, func(r rune) bool { return r == '/' || r == '\\' }) {
		if part == ".." {
			return false
		}
	}
	return true
}

func lintCalibration(dir string, r *Rubric) ([]Problem, []fileBytes) {
	path := filepath.Join(dir, CalibrationFile)
	if _, err := os.Lstat(path); errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	data, err := readRegular(path)
	if err != nil {
		return []Problem{{File: path, Line: 1, Message: "calibration.json is unreadable or a symlink"}}, nil
	}
	files := []fileBytes{{CalibrationFile, data}}
	var c calibrationFile
	if err := json.Unmarshal(data, &c); err != nil {
		return []Problem{{File: path, Line: 1, Message: "calibration.json does not parse: " + err.Error()}}, files
	}
	var out []Problem
	if c.SchemaVersion != SchemaVersion {
		out = append(out, Problem{File: path, Line: 1, Message: fmt.Sprintf("schema_version must be %d", SchemaVersion)})
	}
	if c.Rubric.ID != r.ID {
		out = append(out, Problem{File: path, Line: 1, Message: fmt.Sprintf("calibration is for rubric %q, not %q", c.Rubric.ID, r.ID)})
	}
	if c.Status != "pass" && c.Status != "fail" {
		out = append(out, Problem{File: path, Line: 1, Message: fmt.Sprintf("status %q must be pass or fail", c.Status)})
	}
	return out, files
}

// Findings converts problems to AR9G8 lint findings, paths shown relative to cwd.
func Findings(problems []Problem, cwd string) []lint.Finding {
	out := make([]lint.Finding, 0, len(problems))
	for _, p := range problems {
		file := p.File
		if cwd != "" {
			if rel, err := filepath.Rel(cwd, file); err == nil && !strings.HasPrefix(rel, "..") {
				file = rel
			}
		}
		out = append(out, lint.Finding{
			Code: lint.CodeReviewRubricInvalid, Name: "rubric-invalid", Severity: lint.SeverityError,
			File: file, Line: max(p.Line, 1), Message: p.Message,
		})
	}
	return out
}

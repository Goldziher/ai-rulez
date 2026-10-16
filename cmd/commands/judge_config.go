package commands

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/judge"
	"github.com/Goldziher/ai-rulez/v5/internal/llm"
	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
)

const (
	// judgeMaxPromptBytes bounds a prompt extension, override or rubric file.
	judgeMaxPromptBytes = 64 * 1024
	// judgeDefaultTrapsFile is where repository false-positive shapes live by convention.
	judgeDefaultTrapsFile = ".ai-rulez/judge/traps.yaml"
	// judgeSkillFile and judgeSkillsDir are where a skill's rubric lives: skills/<name>/SKILL.md.
	judgeSkillFile = "SKILL.md"
	judgeSkillsDir = "skills"
	// judgeClaudeMD is the Claude Code instruction file.
	judgeClaudeMD = "CLAUDE.md"

	// The default models per role: the reviewers read a lot and answer in a fixed shape, so a
	// fast model suffices; the adjudicator decides what survives, so it is the strongest one.
	judgeDefaultReviewerModel    = "bedrock/global.anthropic.claude-sonnet-5-5"
	judgeDefaultAdjudicatorModel = "bedrock/global.anthropic.claude-opus-5-5"
)

// judgeSettings is everything a run takes from flags, the repository's [judge] table and the user's.
type judgeSettings struct {
	opts    judge.Options
	skills  []judge.LoadedSkill
	notices []string
	// consent is true when the user config agrees to send content (the flag is the other way).
	consent bool
}

// judgeSkillRef is a rubric skill reference with the categories it applies to.
type judgeSkillRef struct {
	ref  string
	cats []judge.Category
	// projectOnly distinguishes repository references from explicit CLI paths.
	projectOnly bool
	// pin is the SHA-256 prefix the configuration pins the skill to.
	pin string
}

// buildSettings merges flags, the repository's [judge] table (gating, attachment, skills, prompts,
// suppressions) and the user's (models, caps, concurrency, the prompt-override switch). A flag wins.
func (d judgeDeps) buildSettings(f *judgeFlags, fl flagChanged, w *judgeWorkspace, res config.JudgeResolution, lc llm.Config) (*judgeSettings, error) {
	repo, user := res.Repo, res.User
	s := &judgeSettings{}
	if len(res.IgnoredRepoKeys) > 0 {
		s.notices = append(s.notices, fmt.Sprintf("the repository's [judge] table sets %s; these keys choose where content goes and what a run spends, so only your user config, the environment and flags can set them: ignored", strings.Join(res.IgnoredRepoKeys, ", ")))
	}
	o := &s.opts
	if err := applyGating(o, f, repo); err != nil {
		return nil, err
	}
	applyAttachment(o, f, fl, repo)
	o.ExplainDrops = f.explainDrops
	applySpend(s, f, user)
	o.Caps = judge.Caps{MaxCostUSD: lc.MaxCostUSD, MaxTokens: lc.MaxTokens, MaxCalls: lc.MaxCalls}
	var err error
	if o.Models, err = d.roleModels(f, lc, user); err != nil {
		return nil, err
	}
	if err := d.loadSkills(f, w, repo, s); err != nil {
		return nil, err
	}
	if err := d.loadPrompts(f, w, repo, user, res, s); err != nil {
		return nil, err
	}
	if o.Suppress, err = parseSuppressions(d.now(), repo, user); err != nil {
		return nil, err
	}
	o.Clock = d.clock
	return s, nil
}

// applyGating sets the categories, the gate and the confidence floor. The repository's gate entries
// come first and the flag's after, so the flag wins per category.
func applyGating(o *judge.Options, f *judgeFlags, repo *config.JudgeConfig) error {
	catNames := firstNonEmptyList(f.categories, repoList(repo, func(j *config.JudgeConfig) []string { return j.Categories }))
	cats, err := parseJudgeCategories(catNames)
	if err != nil {
		return err
	}
	o.Categories = cats
	var failOn []string
	if repo != nil {
		failOn = append(failOn, repo.FailOn...)
	}
	failOn = append(failOn, f.failOn...)
	if o.FailOn, err = judge.ParseFailOn(failOn); err != nil {
		return oops.Wrapf(err, "gate")
	}
	o.MinConfidence, err = parseMinConfidence(f, repo)
	return err
}

func parseMinConfidence(f *judgeFlags, repo *config.JudgeConfig) (judge.Confidence, error) {
	conf := strings.ToLower(strings.TrimSpace(f.minConfidence))
	if conf == "" && repo != nil {
		conf = strings.ToLower(strings.TrimSpace(repo.MinConfidence))
	}
	switch conf {
	case "", "low":
		return judge.ConfidenceLow, nil
	case "medium":
		return judge.ConfidenceMedium, nil
	case "high":
		return judge.ConfidenceHigh, nil
	}
	return "", oops.Errorf("--min-confidence must be low, medium or high, got %q", conf)
}

func applyAttachment(o *judge.Options, f *judgeFlags, fl flagChanged, repo *config.JudgeConfig) {
	o.Attach = judge.AttachOptions{Rules: f.attachRules, ClaudeMD: f.attachClaudeMD}
	if repo == nil {
		return
	}
	if !fl("attach-rules") {
		o.Attach.Rules = repo.AttachRulesOn()
	}
	if !fl("attach-claude-md") {
		o.Attach.ClaudeMD = repo.AttachClaudeMDOn()
	}
}

// applySpend sets the rate and concurrency: flags, then the user's table.
func applySpend(s *judgeSettings, f *judgeFlags, user *config.JudgeConfig) {
	o := &s.opts
	o.Concurrency, o.RPM = f.concurrency, f.rpm
	if user == nil {
		return
	}
	if o.Concurrency == 0 {
		o.Concurrency = user.Concurrency
	}
	if o.RPM == 0 {
		o.RPM = user.RPM
	}
	s.consent = user.AllowLLM
}

func parseSuppressions(now time.Time, tables ...*config.JudgeConfig) ([]judge.Suppression, error) {
	var out []judge.Suppression
	for _, tbl := range tables {
		if tbl == nil {
			continue
		}
		for _, sc := range tbl.Suppress {
			sup, err := judge.ParseSuppression(sc.Code, sc.Path, sc.Quote, sc.Reason, sc.Expires, now)
			if err != nil {
				return nil, oops.Wrapf(err, "[judge] suppress")
			}
			out = append(out, sup)
		}
	}
	return out, nil
}

// flagChanged reports whether a flag was set on the command line.
type flagChanged func(name string) bool

func repoList(j *config.JudgeConfig, pick func(*config.JudgeConfig) []string) []string {
	if j == nil {
		return nil
	}
	return pick(j)
}

func firstNonEmptyList(lists ...[]string) []string {
	for _, l := range lists {
		if len(l) > 0 {
			return l
		}
	}
	return nil
}

func parseJudgeCategories(names []string) ([]judge.Category, error) {
	var out []judge.Category
	for _, raw := range names {
		for _, n := range strings.Split(raw, ",") {
			if strings.TrimSpace(n) == "" {
				continue
			}
			c, ok := judge.ParseCategory(n)
			if !ok {
				return nil, oops.Errorf("unknown category %q (use security, quality, governance)", n)
			}
			out = append(out, c)
		}
	}
	return out, nil
}

// roleModels picks the model of each role. Precedence: --model-for, --model / --adjudicator-model,
// AI_RULEZ_LLM_MODEL and the user's [llm] model, the user's [judge.models], the defaults.
func (d judgeDeps) roleModels(f *judgeFlags, lc llm.Config, user *config.JudgeConfig) (judge.Models, error) {
	explicitReviewer := f.model
	if explicitReviewer == "" {
		explicitReviewer = lc.FullModel()
	}
	pick := func(roles ...string) string {
		if user != nil {
			for _, r := range roles {
				if m := strings.TrimSpace(user.Models[r]); m != "" {
					return m
				}
			}
		}
		return ""
	}
	m := judge.Models{
		Security:   firstNonEmpty(explicitReviewer, pick("security", "reviewers"), judgeDefaultReviewerModel),
		Quality:    firstNonEmpty(explicitReviewer, pick("quality", "reviewers"), judgeDefaultReviewerModel),
		Governance: firstNonEmpty(explicitReviewer, pick("governance", "reviewers"), judgeDefaultReviewerModel),
	}
	// With an explicit single model the adjudicator uses it too, unless it is named.
	m.Adjudicator = firstNonEmpty(f.adjudicatorModel, pick("adjudicator"), explicitReviewer, judgeDefaultAdjudicatorModel)
	for _, spec := range f.modelFor {
		cat, model, ok := strings.Cut(spec, "=")
		if !ok || strings.TrimSpace(model) == "" {
			return m, oops.Errorf("--model-for wants CATEGORY=MODEL, got %q", spec)
		}
		model = strings.TrimSpace(model)
		switch strings.ToLower(strings.TrimSpace(cat)) {
		case "security":
			m.Security = model
		case "quality":
			m.Quality = model
		case "governance":
			m.Governance = model
		case "adjudicator":
			m.Adjudicator = model
		default:
			return m, oops.Errorf("--model-for: %q is not security, quality, governance or adjudicator", cat)
		}
	}
	return m, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// loadSkills resolves the rubric skills of the flags and of [judge.skills].
func (d judgeDeps) loadSkills(f *judgeFlags, w *judgeWorkspace, repo *config.JudgeConfig, s *judgeSettings) error {
	pins := map[string]string{}
	var refs []judgeSkillRef
	if repo != nil && repo.Skills != nil {
		maps.Copy(pins, repo.Skills.Pins)
		refs = repoSkillRefs(repo.Skills, pins)
	}
	for _, r := range f.skills {
		refs = append(refs, judgeSkillRef{ref: r, cats: []judge.Category{judge.CategoryGovernance}, pin: pins[r]})
	}
	for _, spec := range f.skillFor {
		cat, name, ok := strings.Cut(spec, "=")
		c, valid := judge.ParseCategory(cat)
		if !ok || !valid || strings.TrimSpace(name) == "" {
			return oops.Errorf("--skill-for wants CATEGORY=NAME (or CATEGORY=path:DIR), got %q", spec)
		}
		refs = append(refs, judgeSkillRef{ref: strings.TrimSpace(name), cats: []judge.Category{c}, pin: pins[strings.TrimSpace(name)]})
	}
	if len(refs) == 0 {
		return nil
	}
	return registerRubrics(w, refs, f.forceReference, s)
}

// repoSkillRefs lists the skills of [judge.skills]: the loaded ones apply to governance, the rest to their category.
func repoSkillRefs(sk *config.JudgeSkills, pins map[string]string) []judgeSkillRef {
	var refs []judgeSkillRef
	for _, n := range sk.Load {
		refs = append(refs, judgeSkillRef{ref: n, cats: []judge.Category{judge.CategoryGovernance}, pin: pins[n], projectOnly: true})
	}
	for _, cat := range sortedKeysOf(sk.For) {
		c, ok := judge.ParseCategory(cat)
		if !ok {
			continue
		}
		for _, n := range sk.For[cat] {
			refs = append(refs, judgeSkillRef{ref: n, cats: []judge.Category{c}, pin: pins[n], projectOnly: true})
		}
	}
	return refs
}

// registerRubrics loads each distinct skill once and records it under the categories it applies to.
func registerRubrics(w *judgeWorkspace, refs []judgeSkillRef, forceReference bool, s *judgeSettings) error {
	s.opts.Rubrics = map[judge.Category][]judge.Rubric{}
	cache := map[string]judge.Rubric{}
	for _, r := range refs {
		if dir, ok := strings.CutPrefix(r.ref, "path:"); ok && r.projectOnly {
			file, err := projectJudgeFile(w, filepath.Join(dir, judgeSkillFile))
			if err != nil {
				return oops.Wrapf(err, "repository rubric skill %s is outside the project or unsafe", r.ref)
			}
			// Repository paths resolve from the root even when invoked in a subdirectory.
			// Read exactly the file whose containment was checked above.
			r.ref = "path:" + filepath.Dir(file)
		}
		rub, ok := cache[r.ref]
		if !ok {
			var err error
			if rub, err = w.loadRubric(r.ref, r.pin, forceReference); err != nil {
				return err
			}
			cache[r.ref] = rub
		}
		for _, c := range r.cats {
			if !hasRubric(s.opts.Rubrics[c], rub) {
				s.opts.Rubrics[c] = append(s.opts.Rubrics[c], rub)
			}
			recordLoadedSkill(s, rub, c)
		}
	}
	return nil
}

// recordLoadedSkill adds the skill to the run's provenance once and the category to it once.
func recordLoadedSkill(s *judgeSettings, rub judge.Rubric, c judge.Category) {
	i := slices.IndexFunc(s.skills, func(ls judge.LoadedSkill) bool { return ls.Name == rub.Name && ls.SHA256 == rub.SHA256 })
	if i < 0 {
		s.skills = append(s.skills, judge.LoadedSkill{Name: rub.Name, Path: rub.Path, SHA256: rub.SHA256, Tier: rub.Tier})
		i = len(s.skills) - 1
	}
	if !slices.Contains(s.skills[i].Categories, string(c)) {
		s.skills[i].Categories = append(s.skills[i].Categories, string(c))
	}
}

func hasRubric(list []judge.Rubric, r judge.Rubric) bool {
	for _, x := range list {
		if x.Name == r.Name && x.SHA256 == r.SHA256 {
			return true
		}
	}
	return false
}

func sortedKeysOf[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// loadRubric reads a rubric skill: "path:DIR" is a directory the user named (configured text, T1);
// anything else is a skill name looked up below the project's skills/ directories. A named skill is
// reference data (T2) unless the configuration pins its hash and the hash matches.
func (w *judgeWorkspace) loadRubric(ref, pin string, forceReference bool) (judge.Rubric, error) {
	if dir, ok := strings.CutPrefix(ref, "path:"); ok {
		return w.loadRubricDir(ref, dir, forceReference)
	}
	return w.loadRubricNamed(ref, pin)
}

func (w *judgeWorkspace) loadRubricDir(ref, dir string, forceReference bool) (judge.Rubric, error) {
	abs := dir
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(w.cwd, dir)
	}
	rel, inside := w.rel(abs)
	if forceReference && !inside {
		return judge.Rubric{}, oops.Errorf("rubric skill %s is outside the project; a tool caller may only name skills inside it", ref)
	}
	if inside {
		if link, through := w.throughSymlink(abs); through {
			return judge.Rubric{}, oops.Errorf("rubric skill %s goes through the symlink %s", ref, link)
		}
	}
	data, err := readCapped(filepath.Join(abs, judgeSkillFile), judgeMaxPromptBytes)
	if err != nil {
		return judge.Rubric{}, oops.Wrapf(err, "read the rubric skill %s", ref)
	}
	display := dir
	if inside {
		display = rel
	}
	tier := judge.TierConfigured
	if forceReference {
		tier = judge.TierReference
	}
	r, perr := judge.ParseRubric(filepath.Base(abs), display, data, tier)
	return r, wrapRubricErr(perr, ref)
}

func (w *judgeWorkspace) loadRubricNamed(ref, pin string) (judge.Rubric, error) {
	var matches []string
	for _, c := range w.candidates {
		if path.Base(c) == judgeSkillFile && path.Base(path.Dir(c)) == ref && path.Base(path.Dir(path.Dir(c))) == judgeSkillsDir {
			matches = append(matches, c)
		}
	}
	switch len(matches) {
	case 0:
		return judge.Rubric{}, oops.Hint("Name a skill below a skills/ directory of the project, or pass path:DIR").Errorf("no rubric skill named %q in the project", ref)
	case 1:
	default:
		return judge.Rubric{}, oops.Hint("Pass path:DIR to pick one").Errorf("rubric skill %q is ambiguous: %s", ref, strings.Join(matches, ", "))
	}
	sf := w.readFile(matches[0])
	if sf.Skip != "" || len(sf.Data) > judgeMaxPromptBytes {
		return judge.Rubric{}, oops.Errorf("cannot read the rubric skill %s", matches[0])
	}
	r, err := judge.ParseRubric(ref, matches[0], string(sf.Data), judge.TierReference)
	if err != nil {
		return r, wrapRubricErr(err, ref)
	}
	if pin != "" && r.Pinned(pin) {
		r.Tier = judge.TierConfigured
	}
	return r, nil
}

func wrapRubricErr(err error, ref string) error {
	if err == nil {
		return nil
	}
	return oops.Wrapf(err, "rubric skill %s", ref)
}

// readCapped reads a file without following a symlink, refusing one over limit bytes.
func readCapped(abs string, limit int64) (string, error) {
	f, err := safefs.OpenRegular(abs)
	if err != nil {
		return "", err //nolint:wrapcheck // the caller adds context
	}
	defer f.Close() //nolint:errcheck // read-only
	data, err := safefs.ReadLimited(f, limit)
	if err != nil {
		return "", err //nolint:wrapcheck // the caller adds context
	}
	return string(data), nil
}

// loadPrompts reads the prompt extension and override files of the repository's and the user's
// [judge.prompts] and the repository's false-positive traps. An override from a repository file takes
// effect only with --allow-prompt-override or allow_prompt_override in the user config: a file that
// replaces the reviewer's instructions is the one thing a hostile repository would most like to ship.
func (d judgeDeps) loadPrompts(f *judgeFlags, w *judgeWorkspace, repo, user *config.JudgeConfig, res config.JudgeResolution, s *judgeSettings) error {
	ps := judge.PromptSet{Extend: map[judge.Category]string{}, Override: map[judge.Category]string{}}
	allowOverride := f.allowPromptOverride || (user != nil && user.AllowPromptOverride)
	userDir := ""
	if res.UserFile != "" {
		userDir = filepath.Dir(res.UserFile)
	}
	for _, src := range []struct {
		tbl  *config.JudgeConfig
		repo bool
	}{{repo, true}, {user, false}} {
		if src.tbl == nil {
			continue
		}
		for _, name := range sortedKeysOf(src.tbl.Prompts) {
			cat, ok := judge.ParseCategory(name)
			if !ok {
				continue
			}
			p := src.tbl.Prompts[name]
			text, err := readPromptFile(w, name, p, src.repo, userDir)
			if err != nil {
				return err
			}
			if err := applyPrompt(&ps, cat, name, p.Mode, text, src.repo && !allowOverride); err != nil {
				return err
			}
		}
	}
	if err := loadTraps(&ps, w, repo); err != nil {
		return err
	}
	s.opts.Prompts = ps
	return nil
}

// readPromptFile reads a prompt file: a repository's stays inside the project, a user's resolves
// against the directory of the user config.
func readPromptFile(w *judgeWorkspace, name string, p config.JudgePrompt, fromRepo bool, userDir string) (string, error) {
	var text string
	var err error
	if fromRepo {
		abs, pathErr := projectJudgeFile(w, p.File)
		if pathErr != nil {
			return "", oops.Wrapf(pathErr, "[judge.prompts.%s] file %q", name, p.File)
		}
		text, err = readCapped(abs, judgeMaxPromptBytes)
	} else {
		file := p.File
		if !filepath.IsAbs(file) {
			file = filepath.Join(userDir, file)
		}
		text, err = readCapped(file, judgeMaxPromptBytes)
	}
	if err != nil {
		return "", oops.Wrapf(err, "read the %s prompt file %s", name, p.File)
	}
	return text, nil
}

// applyPrompt records a prompt as an extension or, when its mode says so and nothing forbids it, an override.
func applyPrompt(ps *judge.PromptSet, cat judge.Category, name, mode, text string, overrideForbidden bool) error {
	if mode != config.JudgePromptOverride {
		ps.Extend[cat] = strings.TrimSpace(ps.Extend[cat] + "\n\n" + text)
		return nil
	}
	if overrideForbidden {
		return oops.Hint("Pass --allow-prompt-override (or set allow_prompt_override in your user config) if you trust this repository's prompt, or use mode = \"extend\"").
			Errorf("[judge.prompts.%s] replaces the reviewer's role prompt; a repository prompt may only extend it unless you allow an override with --allow-prompt-override", name)
	}
	ps.Override[cat] = text
	return nil
}

// loadTraps reads the repository's false-positive traps; a missing file is fine unless the config names one.
func loadTraps(ps *judge.PromptSet, w *judgeWorkspace, repo *config.JudgeConfig) error {
	trapsFile := judgeDefaultTrapsFile
	if repo != nil && strings.TrimSpace(repo.TrapsFile) != "" {
		trapsFile = repo.TrapsFile
	}
	abs, err := projectJudgeFile(w, trapsFile)
	if err != nil {
		return oops.Wrapf(err, "traps_file %s", trapsFile)
	}
	data, err := readCapped(abs, judgeMaxPromptBytes)
	if err != nil {
		if repo != nil && repo.TrapsFile != "" && !errors.Is(err, os.ErrNotExist) {
			return oops.Wrapf(err, "read traps_file %s", trapsFile)
		}
		return nil
	}
	traps, perr := judge.ParseRepoTraps([]byte(data))
	if perr != nil {
		return oops.Wrapf(perr, "read %s", trapsFile)
	}
	ps.RepoTraps = traps
	return nil
}

// projectJudgeFile confines repository-controlled prompt inputs before any read.
func projectJudgeFile(w *judgeWorkspace, file string) (string, error) {
	if filepath.IsAbs(file) {
		return "", oops.Errorf("file must be a relative path inside the project")
	}
	abs := filepath.Join(w.root, filepath.FromSlash(file))
	if rel, inside := w.rel(abs); !inside || rel == "." {
		return "", oops.Errorf("file must stay inside the project")
	}
	if link, through := w.throughSymlink(abs); through {
		return "", oops.Errorf("file must stay inside the project without following the symlink %s", link)
	}
	return abs, nil
}

// judgeKeyHome says where a key that does not belong in a repository's [judge] table is set; ""
// means the key is simply unknown.
const keyModel = "model"

func judgeKeyHome(key string) string {
	switch key {
	case keyModel:
		return "the --model flag, or [judge.models] in your user config"
	case "provider":
		return "the --provider flag or [llm] provider in your user config"
	case "base_url":
		return "[llm] base_url in your user config (or AI_RULEZ_LLM_BASE_URL)"
	case "api_key_env":
		return "[llm] api_key_env in your user config"
	case "api_key":
		return "the environment variable named by [llm] api_key_env in your user config"
	}
	return ""
}

// repoConfigNotices reports what the repository configuration sets that judge ignores: user-only keys
// of its [judge] table (the schema calls them unknown, as `validate` does) and the provider, model and
// endpoint of its [llm] table. A key of [judge] that is unknown for any other reason is an error, as
// it is for `validate`: a misspelled gate would otherwise silently weaken the run.
func repoConfigNotices(cfg *config.Config) (notices []string, err error) {
	if cfg == nil {
		return nil, nil
	}
	findings, ferr := config.SchemaFindings(cfg)
	if ferr != nil {
		return nil, ferr //nolint:wrapcheck // already contextual
	}
	var misplaced, unknown []string
	for _, f := range findings {
		if f.Path != "judge" || f.Key == "" {
			continue
		}
		if judgeKeyHome(f.Key) != "" {
			misplaced = append(misplaced, f.Key)
		} else {
			unknown = append(unknown, f.String())
		}
	}
	if len(unknown) > 0 {
		return nil, oops.Errorf("invalid [judge] table: %s", strings.Join(unknown, "; "))
	}
	for _, k := range misplaced {
		notices = append(notices, fmt.Sprintf("the repository's [judge] table sets %s, which is not a [judge] key and has no effect: set it with %s", k, judgeKeyHome(k)))
	}
	if l := cfg.LLM; l != nil {
		var set []string
		for _, kv := range []struct{ key, val string }{{"provider", l.Provider}, {keyModel, l.Model}, {"base_url", l.BaseURL}} {
			if kv.val != "" {
				set = append(set, kv.key)
			}
		}
		if len(set) > 0 {
			notices = append(notices, fmt.Sprintf("the repository's [llm] table sets %s; judge never reads a repository's model settings, because a repository must not choose where content goes: set them in your user config or with flags", strings.Join(set, ", ")))
		}
	}
	return notices, nil
}

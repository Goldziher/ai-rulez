package policy

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/semver"
)

// SizeBudget bounds the size of one content kind ([lint.budgets.<kind>]). A zero
// field is no bound.
type SizeBudget struct {
	MaxLines  int
	MaxTokens int
}

type fileSizeBudget struct {
	MaxLines  *int `toml:"max_lines"`
	MaxTokens *int `toml:"max_tokens"`
}

func parseSizeBudgets(in map[string]fileSizeBudget) (map[string]SizeBudget, error) {
	known := lint.SizeBudgetKinds()
	var out map[string]SizeBudget
	for kind, b := range in {
		if !slices.Contains(known, kind) {
			return nil, fmt.Errorf("lint.budgets: unknown content kind %q (the policy is newer than this ai-rulez, or the name is wrong; known: %s)", kind, strings.Join(known, ", "))
		}
		var sb SizeBudget
		for name, v := range map[string]struct {
			in  *int
			out *int
		}{"max_lines": {b.MaxLines, &sb.MaxLines}, "max_tokens": {b.MaxTokens, &sb.MaxTokens}} {
			if v.in == nil {
				continue
			}
			if *v.in < 1 {
				return nil, fmt.Errorf("lint.budgets.%s.%s: %d must be positive", kind, name, *v.in)
			}
			*v.out = *v.in
		}
		if out == nil {
			out = map[string]SizeBudget{}
		}
		out[kind] = sb
	}
	return out, nil
}

// mergeSizeBudgets keeps the lower positive bound of each kind and field.
func mergeSizeBudgets(a, b map[string]SizeBudget) map[string]SizeBudget {
	if len(a) == 0 && len(b) == 0 {
		return nil
	}
	out := map[string]SizeBudget{}
	for _, m := range []map[string]SizeBudget{a, b} {
		for kind, sb := range m {
			cur := out[kind]
			cur.MaxLines = lowerPositive(cur.MaxLines, sb.MaxLines)
			cur.MaxTokens = lowerPositive(cur.MaxTokens, sb.MaxTokens)
			out[kind] = cur
		}
	}
	return out
}

// lowerPositive is the meet of two upper bounds where 0 is no bound.
func lowerPositive(a, b int) int {
	switch {
	case a == 0:
		return b
	case b == 0:
		return a
	}
	return min(a, b)
}

func addSizeBudgets(budgets map[string]SizeBudget, table func(path ...string) map[string]any) {
	for kind, sb := range budgets {
		if sb.MaxLines > 0 {
			table("lint", "budgets", kind)["max_lines"] = sb.MaxLines
		}
		if sb.MaxTokens > 0 {
			table("lint", "budgets", kind)["max_tokens"] = sb.MaxTokens
		}
	}
}

// sizeBudgets clamps [lint.budgets] to the policy's per-kind bounds. A lower
// repository value is a narrowing; an unset one runs at the built-in budget, and
// a policy bound above that must not raise it.
func (a *applier) sizeBudgets() {
	pol := a.res.Policy.Lint.SizeBudgets
	if len(pol) == 0 {
		return
	}
	if a.cfg.Lint == nil {
		a.cfg.Lint = &config.LintConfig{}
	}
	lc := a.cfg.Lint
	for _, kind := range sortedBudgetKinds(pol) {
		bound := pol[kind]
		have := lc.Budgets[kind]
		def := lint.DefaultSizeBudget(kind)
		fields := []struct {
			name       string
			bound, def int
			have       *int
		}{
			{"max_lines", bound.MaxLines, def.MaxLines, &have.MaxLines},
			{"max_tokens", bound.MaxTokens, def.MaxTokens, &have.MaxTokens},
		}
		for _, f := range fields {
			if f.bound == 0 {
				continue
			}
			key := "lint.budgets." + kind + "." + f.name
			switch {
			case *f.have > f.bound:
				a.violate(lint.CodePolicyLoosened, key, f.name,
					"[lint.budgets.%s] %s = %d is above the policy limit %d (origin: %s); %d is enforced", kind, f.name, *f.have, f.bound, a.origin(key), f.bound)
				*f.have = f.bound
			case *f.have > 0 && *f.have < f.bound:
				a.accept = append(a.accept, fmt.Sprintf("%s (lowered to %d)", key, *f.have))
			case *f.have == 0:
				*f.have = f.bound
				if f.def > 0 && f.def < f.bound {
					*f.have = f.def
				}
			}
		}
		if lc.Budgets == nil {
			lc.Budgets = map[string]config.LintBudget{}
		}
		lc.Budgets[kind] = have
	}
}

func sortedBudgetKinds(m map[string]SizeBudget) []string {
	kinds := make([]string, 0, len(m))
	for k := range m {
		kinds = append(kinds, k)
	}
	slices.Sort(kinds)
	return kinds
}

// parseMinReleaseAge reads sources.min_release_age with the parser the lock uses.
func parseMinReleaseAge(raw string) (time.Duration, error) {
	age, err := semver.ParseAge(raw)
	if err != nil {
		return 0, fmt.Errorf("sources.min_release_age: %w", err)
	}
	return age, nil
}

// minReleaseAge raises [lock] min_release_age and each source's own to the policy
// floor. An older age is a narrowing; a younger or explicitly absent ("0") one is
// reported and raised. A source that sets none defers to [lock], which is raised.
func (a *applier) minReleaseAge() {
	floor := a.res.Policy.Sources.MinReleaseAge
	if floor <= 0 {
		return
	}
	text := formatAge(floor)
	raise := func(key, needle, where, have string) bool {
		age, err := semver.ParseAge(have)
		if err == nil && age >= floor {
			if age > floor {
				a.accept = append(a.accept, fmt.Sprintf("%s (raised to %s)", key, have))
			}
			return false
		}
		if have != "" && err == nil {
			a.violate(lint.CodePolicyLoosened, key, needle,
				"%s min_release_age = %q is younger than the policy floor %s (origin: %s); %s is enforced", where, have, text, a.origin("sources.min_release_age"), text)
		}
		return true
	}
	if a.cfg.Lock == nil {
		a.cfg.Lock = &config.LockConfig{}
	}
	if raise("lock.min_release_age", "min_release_age", "[lock]", strings.TrimSpace(a.cfg.Lock.MinReleaseAge)) {
		a.cfg.Lock.MinReleaseAge = text
	}
	for i := range a.cfg.Includes {
		s := &a.cfg.Includes[i]
		if s.MinReleaseAge != "" && raise("includes."+s.Name+".min_release_age", s.Name, fmt.Sprintf("include %q", s.Name), strings.TrimSpace(s.MinReleaseAge)) {
			s.MinReleaseAge = text
		}
	}
	for i := range a.cfg.InstalledSkills {
		s := &a.cfg.InstalledSkills[i]
		if s.MinReleaseAge != "" && raise("installed_skills."+s.Name+".min_release_age", s.Name, fmt.Sprintf("installed skill %q", s.Name), strings.TrimSpace(s.MinReleaseAge)) {
			s.MinReleaseAge = text
		}
	}
	for i := range a.cfg.SkillSources {
		s := &a.cfg.SkillSources[i]
		if s.MinReleaseAge != "" && raise("skill_sources."+s.Name+".min_release_age", s.Name, fmt.Sprintf("skill source %q", s.Name), strings.TrimSpace(s.MinReleaseAge)) {
			s.MinReleaseAge = text
		}
	}
}

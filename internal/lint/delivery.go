package lint

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/includes"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/skillsource"
)

// Dynamic skill loading rules (AR990 to AR995). The numbers are reserved for
// this family; the roles work uses AR971 to AR973 and AR981 to AR982.
const (
	CodeServedReferencedStatically = "AR990"
	CodeDeliveryStubMissing        = "AR991"
	CodeDeliveryStaticFallback     = "AR992"
	CodeServedNoServer             = "AR993"
	CodeDeliveryInvalid            = "AR994"
	CodeServedLockMismatch         = "AR995"
)

func registerDelivery(s *ruleSet) {
	s.addRules(
		RuleInfo{CodeServedReferencedStatically, "served-skill-referenced-statically", SeverityWarning, "a static rule, context or skill names a skill whose delivery is served, which is not in the harness's skill tree"},
		RuleInfo{CodeDeliveryStubMissing, "delivery-stub-missing", SeverityError, "skills are served but a harness that can call MCP has no dynamic-skills stub telling the agent to call find_skill"},
		RuleInfo{CodeDeliveryStaticFallback, "delivery-static-fallback", SeverityWarning, "a harness without MCP support keeps served skills as static files (nothing is dropped)"},
		RuleInfo{CodeServedNoServer, "served-no-server", SeverityWarning, "skills are served but no [[mcp_servers]] entry runs `ai-rulez mcp --serve-skills`"},
		RuleInfo{CodeDeliveryInvalid, "delivery-invalid", SeverityError, "a skill's delivery frontmatter is not static, served or both"},
		RuleInfo{CodeServedLockMismatch, "served-lock-mismatch", SeverityError, "[lock] enforce is on and a served skill is not pinned in ai-rulez.lock with its current digest"},
	)
}

// DeliveryFinding is a delivery problem computed outside the lint package (it
// needs the generator): the stub check, the harness fallbacks, the server check
// and the lock check. Each is reported against the configuration file.
type DeliveryFinding struct {
	Code    string
	Message string
	// Severity overrides the rule's configured severity when set.
	Severity Severity
}

// WithDelivery supplies the delivery findings computed by the caller.
func WithDelivery(findings []DeliveryFinding) Option {
	return func(r *runner) { r.delivery = findings }
}

// checkDelivery runs the delivery rules the lint package can decide alone
// (AR990, AR994, AR010 for skill sources) and reports the supplied findings.
func (r *runner) checkDelivery() {
	served := r.servedSkillNames()
	r.checkDeliveryValues()
	r.checkServedReferences(served)
	r.checkUnpinnedSources()
	path := r.configFilePath()
	if path == "" {
		path = filepath.Join(r.rootAbs(), ".ai-rulez", "config.toml")
	}
	for _, f := range r.delivery {
		r.addWithSeverity(f.Code, f.Severity, path, 1, "%s", f.Message)
	}
}

// servedSkillNames lists the skills whose delivery is served (not both).
func (r *runner) servedSkillNames() map[string]bool {
	out := map[string]bool{}
	for i := range r.items {
		it := &r.items[i]
		if it.kind == kindSkill && !it.isDoc && r.cfg.EffectiveDelivery(it.cf, it.domain, nil) == config.DeliveryServed {
			out[strings.ToLower(itemID(kindSkill, it.cf))] = true
		}
	}
	return out
}

func (r *runner) checkDeliveryValues() {
	for i := range r.items {
		it := &r.items[i]
		if it.kind != kindSkill || it.isDoc || !it.owned {
			continue
		}
		v := config.SkillDeliveryValue(it.cf)
		if v == "" {
			continue
		}
		if _, ok := config.ParseDelivery(v); ok {
			continue
		}
		line := 1
		if k, found := parseFrontmatterDoc(r.docs[it.abs]).top("delivery"); found {
			line = k.Line
		}
		r.add(CodeDeliveryInvalid, it.abs, line, "delivery %q is not one of static, served, both; it is ignored and the skill keeps its inherited delivery", v)
	}
}

// checkServedReferences warns where static content names a served skill: the
// harness cannot see that skill in its tree, so the reference points at nothing
// until the agent calls find_skill.
func (r *runner) checkServedReferences(served map[string]bool) {
	if len(served) == 0 {
		return
	}
	for i := range r.items {
		it := &r.items[i]
		if !it.owned {
			continue
		}
		if it.kind == kindSkill && !it.isDoc && served[strings.ToLower(itemID(kindSkill, it.cf))] {
			continue // served content may refer to other served content
		}
		if it.kind == kindSkill && it.isDoc && it.itemDir != "" && r.resourceOfServed(it, served) {
			continue
		}
		d, ok := r.docs[it.abs]
		if !ok {
			continue
		}
		reported := map[string]bool{}
		for _, l := range d.body() {
			for _, name := range referencedNames(l, served) {
				if reported[name] {
					continue
				}
				reported[name] = true
				r.add(CodeServedReferencedStatically, it.abs, l.No,
					"%s is served, not written to the harness's skill tree; this static %s refers to it by name. Load it with find_skill/load_skill, or set delivery: both on %s", name, it.kind, name)
			}
		}
	}
}

func (r *runner) resourceOfServed(doc *item, served map[string]bool) bool {
	for i := range r.items {
		o := &r.items[i]
		if o.kind == kindSkill && !o.isDoc && o.itemDir == doc.itemDir && served[strings.ToLower(itemID(kindSkill, o.cf))] {
			return true
		}
	}
	return false
}

// referencedNames returns the served skill names a line refers to by the same
// forms the reference check (AR301) understands.
func referencedNames(l bodyLine, served map[string]bool) []string {
	found := map[string]bool{}
	for _, m := range nameAfterRe.FindAllStringSubmatch(l.Text, -1) {
		if m[2] == kindSkill {
			found[m[1]] = true
		}
	}
	for _, m := range nameBeforeRe.FindAllStringSubmatch(l.Text, -1) {
		if m[1] == kindSkill {
			found[m[2]] = true
		}
	}
	for _, m := range skillCallRe.FindAllStringSubmatch(l.Text, -1) {
		found[m[1]] = true
	}
	for _, m := range slashRe.FindAllStringSubmatchIndex(l.Plain, -1) {
		if slashInvocation(l.Plain[:m[2]-1]) {
			found[l.Plain[m[2]:m[3]]] = true
		}
	}
	for _, m := range backtickRe.FindAllStringSubmatch(l.Text, -1) {
		found[strings.TrimPrefix(strings.TrimSpace(m[1]), "/")] = true
	}
	var out []string
	for name := range found {
		if served[strings.ToLower(name)] {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// checkUnpinnedSources reports AR010 for [[skill_sources]] that follow a moving
// ref (a branch, a tag or nothing) without a pin in ai-rulez.lock. A full commit
// SHA counts as pinned.
func (r *runner) checkUnpinnedSources() {
	if len(r.cfg.SkillSources) == 0 {
		return
	}
	lock, _ := lockfile.Load(r.cfg.ConfigDir) //nolint:errcheck // an unreadable lock pins nothing
	path := r.configFilePath()
	if path == "" {
		path = filepath.Join(r.rootAbs(), ".ai-rulez", "config.toml")
	}
	for i := range r.cfg.SkillSources {
		src := &r.cfg.SkillSources[i]
		if !includes.IsGitURL(strings.TrimPrefix(src.URL, "git+")) {
			continue
		}
		want := skillsource.FromConfig(src).Want()
		if lockfile.IsFullSHA(want.Ref) || lock.Find(lockfile.KindSource, src.Name).Covers(want) {
			continue
		}
		line := 1
		if data, err := os.ReadFile(path); err == nil {
			for n, l := range strings.Split(string(data), "\n") {
				if strings.Contains(l, `"`+src.Name+`"`) {
					line = n + 1
					break
				}
			}
			if _, ok := r.docs[path]; !ok {
				r.docs[path] = doc{lines: strings.Split(string(data), "\n")}
			}
		}
		ref := src.RequestedRef()
		if ref == "" {
			ref = "the default branch (HEAD)"
		}
		r.add(CodeUnpinnedRemote, path, line, "skill source %q follows %s and is not pinned by %s; run `ai-rulez lock` (or pin ref to a full commit SHA)", src.Name, ref, lockfile.FileName)
	}
}

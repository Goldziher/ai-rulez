package lint

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/includes"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/skillsource"
)

// Dynamic skill loading rules. AR990 to AR995 are the first delivery block; the
// roles work uses AR971 to AR973 and AR981 to AR982. AR9R0 to AR9R2 are the
// on-disk and invocation traps that serving a skill introduces (docs/strict-validation.md).
const (
	CodeServedReferencedStatically = "AR990"
	CodeDeliveryStubMissing        = "AR991"
	CodeDeliveryStaticFallback     = "AR992"
	CodeServedNoServer             = "AR993"
	CodeDeliveryInvalid            = "AR994"
	CodeServedLockMismatch         = "AR995"
	// CodeServedBundledFiles warns that a served skill ships scripts/ or assets/
	// that are never written to disk, so a relative path into them resolves to
	// nothing.
	CodeServedBundledFiles = "AR9R0"
	// CodeBothDMIPointer warns that always-loaded content points at a delivery:
	// both skill whose frontmatter sets disable-model-invocation: true; the Skill
	// tool refuses it, so a bare pointer leaves the model declining the workflow.
	CodeBothDMIPointer = "AR9R1"
	// CodeServedDMIUnreachable warns that a delivery: served skill sets
	// disable-model-invocation: true: it is never written to disk, so the user has
	// no slash command for it, and the skills server hides it from the model.
	CodeServedDMIUnreachable = "AR9R2"
)

func registerDelivery(s *ruleSet) {
	s.addRules(
		RuleInfo{CodeServedReferencedStatically, "served-skill-referenced-statically", SeverityWarning, "an always-loaded rule, context, agent or command names a skill whose delivery is served, which is not in the harness's skill tree; a served skill naming a sibling served skill is fine"},
		RuleInfo{CodeDeliveryStubMissing, "delivery-stub-missing", SeverityError, "skills are served but a harness that can call MCP has no dynamic-skills stub telling the agent to call find_skill"},
		RuleInfo{CodeDeliveryStaticFallback, "delivery-static-fallback", SeverityWarning, "a harness without MCP support keeps served skills as static files (nothing is dropped)"},
		RuleInfo{CodeServedNoServer, "served-no-server", SeverityWarning, "skills are served but no MCP server runs `ai-rulez mcp --serve-skills`; the generated self entry runs it when mcp.self_server is on"},
		RuleInfo{CodeDeliveryInvalid, "delivery-invalid", SeverityError, "a skill's delivery frontmatter is not static, served or both"},
		RuleInfo{CodeServedLockMismatch, "served-lock-mismatch", SeverityError, "[lock] enforce is on and a served skill is not pinned in ai-rulez.lock with its current digest"},
		RuleInfo{CodeServedBundledFiles, "served-skill-bundled-files", SeverityWarning, "a served skill ships scripts/ or assets/ that are not written to disk, so a relative path into them does not resolve; set delivery: both to keep them"},
		RuleInfo{CodeBothDMIPointer, "both-dmi-pointer", SeverityWarning, "always-loaded content points at a delivery: both skill whose frontmatter sets disable-model-invocation: true, by a bare name or Skill(...); the Skill tool refuses it and the model then declines the workflow"},
		RuleInfo{CodeServedDMIUnreachable, "served-dmi-unreachable", SeverityWarning, "a delivery: served skill sets disable-model-invocation: true, so nobody can run it: it is not written to disk (no slash command) and the skills server hides it from the model; set delivery: both, or [mcp] serve_user_invoked_skills = true"},
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
	r.checkServedBundledFiles(served)
	r.checkBothDMIPointers()
	r.checkServedDMIUnreachable()
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

// checkServedReferences warns where always-loaded content names a served skill:
// the harness cannot see that skill in its tree, so the reference points at
// nothing until the agent calls find_skill.
//
// Only rules, context, agents and commands are checked. A skill is loaded on
// demand, so a served skill that names a sibling served skill is fine: the
// agent loads it through find_skill and follows the reference there. Reporting
// that case made every served skill name its neighbors, and silencing it by
// forcing one skill static made the next one fire until the whole domain was
// static (#319).
func (r *runner) checkServedReferences(served map[string]bool) {
	if len(served) == 0 {
		return
	}
	for i := range r.items {
		it := &r.items[i]
		if !it.owned || it.isDoc || !alwaysLoaded(it) {
			continue
		}
		d, ok := r.docs[it.abs]
		if !ok {
			continue
		}
		reported := map[string]bool{}
		r.checkServedFrontmatter(it, d, served, reported)
		r.checkServedBody(it, d, served, reported)
	}
}

// alwaysLoaded reports whether an item's body is loaded whenever the harness
// runs, as opposed to a skill, which is loaded on demand. AR990 concerns
// always-loaded content only.
func alwaysLoaded(it *item) bool {
	switch it.kind {
	case kindRule, kindContext, kindAgent, kindCommand:
		return true
	}
	return false
}

func (r *runner) checkServedFrontmatter(it *item, d doc, served, reported map[string]bool) {
	if it.isDoc || it.cf.Metadata == nil {
		return
	}
	for _, s := range it.cf.Metadata.Skills {
		key := strings.ToLower(strings.TrimSpace(s))
		if !served[key] || reported[key] {
			continue
		}
		reported[key] = true
		r.add(CodeServedReferencedStatically, it.abs, d.lineOf(s, 1),
			"frontmatter skills: preloads %q, which is served (%s) and not written to the harness's skill tree. The transitive fix is to set delivery: both on %s, or drop it from skills:", s, r.servedSkillSource(key), s)
	}
}

func (r *runner) checkServedBody(it *item, d doc, served, reported map[string]bool) {
	for _, l := range d.body() {
		for _, name := range referencedNames(l, served) {
			if reported[name] {
				continue
			}
			reported[name] = true
			if servedPathPointer(l, name) {
				r.add(CodeServedReferencedStatically, it.abs, l.No,
					"%s is served (%s), so its file is not written to the harness's skill tree; this %s points at its path, and the file will not be there. The transitive fix is to set delivery: both on %s — it stays in the static tree and the path resolves — or change %s. Reach it with `load_skill %s` instead.",
					name, r.servedSkillSource(name), it.kind, name, r.servedSkillSource(name), name)
				continue
			}
			r.add(CodeServedReferencedStatically, it.abs, l.No,
				"%s is served (%s), so it is not in the harness's skill tree; this %s refers to it by name. The transitive fix is to set delivery: both on %s — it stays in the static tree and every reference to it resolves — or change %s. Reach it with `load_skill %s` instead.",
				name, r.servedSkillSource(name), it.kind, name, r.servedSkillSource(name), name)
		}
	}
}

// servedPathPointer reports whether a line names the skill by file path
// (skills/<name>, .claude/skills/<name>/...) rather than by bare name. It reuses
// skillDirRe, the file-path matcher referenceNames already runs, so AR990 can say
// the file will not be there instead of only that the name points nowhere.
func servedPathPointer(l bodyLine, name string) bool {
	if !strings.Contains(l.Text, "skills/") {
		return false
	}
	for _, m := range skillDirRe().FindAllStringSubmatch(l.Text, -1) {
		if strings.EqualFold(m[1], name) {
			return true
		}
	}
	return false
}

// checkServedBundledFiles warns (AR9R0) when a served skill ships files under
// scripts/ or assets/. A served skill reaches the agent over MCP, so a relative
// path into a script or asset — the form SKILL.md usually uses — has no file on
// disk to resolve. references are left out: they are readable over MCP through
// list_skill_resources/load_skill, so they are not the trap this rule covers.
// Scripts and assets can be fetched too (read_skill_file, with their mode), but
// still have to be written out before they run, which the message says.
func (r *runner) checkServedBundledFiles(served map[string]bool) {
	for i := range r.items {
		it := &r.items[i]
		if it.kind != kindSkill || it.isDoc || !it.owned || !served[strings.ToLower(itemID(kindSkill, it.cf))] {
			continue
		}
		kinds := bundledKinds(it.cf.Resources)
		if len(kinds) == 0 {
			continue
		}
		r.add(CodeServedBundledFiles, it.abs, r.deliveryLine(it),
			"%s is served, so its bundled %s are not written to any skill tree and a relative path into them does not resolve. Set delivery: both to keep the files on disk, or fetch them over MCP with read_skill_file (base64 for binary) and restore the mode that list_skill_resources reports",
			itemID(kindSkill, it.cf), strings.Join(kinds, "/"))
	}
}

// bundledKinds lists, in canonical order, the kinds of bundled file a served
// skill ships that a relative path would need on disk: scripts and assets.
func bundledKinds(resources []config.SkillResource) []string {
	seen := map[string]bool{}
	var out []string
	for _, res := range resources {
		switch res.Kind {
		case config.SkillKindScripts, config.SkillKindAssets:
			if !seen[res.Kind] {
				seen[res.Kind] = true
				out = append(out, res.Kind)
			}
		}
	}
	sort.Strings(out)
	return out
}

// deliveryLine returns the 1-based line of the skill's `delivery` frontmatter key,
// or 1 when it has none.
func (r *runner) deliveryLine(it *item) int {
	if k, found := parseFrontmatterDoc(r.docs[it.abs]).top("delivery"); found {
		return k.Line
	}
	return 1
}

// checkBothDMIPointers warns (AR9R1) where always-loaded content points at a
// skill that is delivery: both and sets disable-model-invocation: true. Such a
// skill is on disk but hidden from the Skill tool, which refuses it; a bare
// pointer ("run the `x` skill", Skill(x)) then leaves the model declining the
// workflow. A file-path pointer still works, so it is not reported. A
// load_skill/find_skill pointer works only when [mcp] serve_user_invoked_skills
// is on: otherwise the skills server hides a user-only skill from the model too.
func (r *runner) checkBothDMIPointers() {
	targets := r.bothDMISkillNames()
	if len(targets) == 0 {
		return
	}
	mcpServes := r.servesUserInvokedSkills()
	for i := range r.items {
		it := &r.items[i]
		if !it.owned || it.isDoc || !alwaysLoaded(it) {
			continue
		}
		d, ok := r.docs[it.abs]
		if !ok {
			continue
		}
		reported := map[string]bool{}
		for _, l := range d.body() {
			if mcpServes && mcpSkillPointer(l.Text) {
				continue
			}
			for _, ref := range referenceNames(l) {
				if ref.kind != kindSkill {
					continue
				}
				key := strings.ToLower(ref.name)
				if !targets[key] || reported[key] || servedPathPointer(l, ref.name) {
					continue
				}
				reported[key] = true
				r.add(CodeBothDMIPointer, it.abs, l.No,
					"%s is delivery: both with disable-model-invocation: true, so the Skill tool refuses it; this %s's pointer by name leaves the model declining the workflow. Point at the file path (the file is on disk), or set [mcp] serve_user_invoked_skills = true and use load_skill, or drop disable-model-invocation",
					ref.name, it.kind)
			}
		}
	}
}

// servesUserInvokedSkills reports [mcp] serve_user_invoked_skills: the skills
// server then serves a disable-model-invocation skill to the model.
func (r *runner) servesUserInvokedSkills() bool {
	m := r.cfg.MCP
	return m != nil && m.ServeUserInvokedSkills != nil && *m.ServeUserInvokedSkills
}

// checkServedDMIUnreachable warns (AR9R2) about a delivery: served skill that
// sets disable-model-invocation: true. A served skill is never written to disk,
// so the user has no slash command for it, and the skills server keeps a
// user-only skill from the model; nobody can run it. Silent when
// [mcp] serve_user_invoked_skills is on, which serves it to the model.
func (r *runner) checkServedDMIUnreachable() {
	if r.servesUserInvokedSkills() {
		return
	}
	for i := range r.items {
		it := &r.items[i]
		if it.kind != kindSkill || it.isDoc || !it.owned {
			continue
		}
		if r.cfg.EffectiveDelivery(it.cf, it.domain, nil) != config.DeliveryServed {
			continue
		}
		fm := parseFrontmatterDoc(r.docs[it.abs])
		if !modelInvocationDisabled(fm) {
			continue
		}
		line := 1
		for _, key := range []string{keyDisableModel, keyDisableModelUnderscore} {
			if k, found := fm.top(key); found {
				line = k.Line
				break
			}
		}
		r.add(CodeServedDMIUnreachable, it.abs, line,
			"%s is delivery: served with disable-model-invocation: true, so nobody can run it: a served skill is never written to disk (no slash command) and the skills server hides it from the model. Set delivery: both, or [mcp] serve_user_invoked_skills = true, or drop disable-model-invocation",
			itemID(kindSkill, it.cf))
	}
}

// bothDMISkillNames lists the lower-case names (id, name, aliases) of skills
// whose delivery is both and whose frontmatter stops the model from invoking
// them. A both skill is on disk, so AR990 does not cover it.
func (r *runner) bothDMISkillNames() map[string]bool {
	out := map[string]bool{}
	for i := range r.items {
		it := &r.items[i]
		if it.kind != kindSkill || it.isDoc || !it.owned {
			continue
		}
		if r.cfg.EffectiveDelivery(it.cf, it.domain, nil) != config.DeliveryBoth {
			continue
		}
		if !modelInvocationDisabled(parseFrontmatterDoc(r.docs[it.abs])) {
			continue
		}
		for _, n := range contentNames(kindSkill, it.cf) {
			out[n] = true
		}
	}
	return out
}

// modelInvocationDisabled reports whether a skill's frontmatter sets
// disable-model-invocation (or its underscore misspelling) to true. Only this
// Claude Code key is read: the trap is the Skill tool refusing the skill.
func modelInvocationDisabled(fm frontmatter) bool {
	for _, name := range []string{keyDisableModel, keyDisableModelUnderscore} {
		if k, ok := fm.top(name); ok && isTrue(k.Value) {
			return true
		}
	}
	return false
}

// mcpSkillPointer reports whether a line already routes the agent through the
// skills MCP server (load_skill, find_skill, list_skill_resources, ...), which
// reaches a disable-model-invocation skill without the Skill tool.
func mcpSkillPointer(text string) bool {
	for _, tool := range []string{"load_skill", "find_skill", "list_skill_resources", "read_skill_file", "get_skill"} {
		if strings.Contains(text, tool) {
			return true
		}
	}
	return false
}

// servedSkillSource names where a skill's served delivery comes from, so the
// AR990 message can point at the setting to change rather than only the skill.
// A value it cannot attribute (a per-role delivery) reads as "the configured
// delivery".
func (r *runner) servedSkillSource(name string) string {
	lower := strings.ToLower(strings.TrimSpace(name))
	for i := range r.items {
		it := &r.items[i]
		if it.kind != kindSkill || it.isDoc || strings.ToLower(itemID(kindSkill, it.cf)) != lower {
			continue
		}
		if v := strings.TrimSpace(config.SkillDeliveryValue(it.cf)); v != "" {
			return fmt.Sprintf("delivery: %s in its frontmatter", v)
		}
		if it.domain != "" {
			if dc, ok := r.cfg.DomainSettings[it.domain]; ok {
				if _, valid := config.ParseDelivery(dc.Delivery); valid {
					return fmt.Sprintf("[domains.%s] delivery = %q", it.domain, dc.Delivery)
				}
			}
		}
		if r.cfg.Skills != nil {
			if _, valid := config.ParseDelivery(r.cfg.Skills.Delivery); valid {
				return fmt.Sprintf("[skills] delivery = %q", r.cfg.Skills.Delivery)
			}
		}
		break
	}
	return "the configured delivery"
}

// loadSkillSpanRe matches a code span that is exactly a load_skill call naming a
// skill, the form AR990 recommends, so the name inside it is not a static
// pointer (#377).
var loadSkillSpanRe = sync.OnceValue(func() *regexp.Regexp {
	return regexp.MustCompile("`load_skill\\s+[\"']?[a-z][a-z0-9-]*[\"']?`")
})

// referencedNames returns the served skill names a line refers to, using the
// same matcher as the reference check (AR301) plus the file-path forms
// (skills/x, .claude/skills/x/SKILL.md). A served skill naming a sibling served
// skill is not reported: only always-loaded content reaches this function.
func referencedNames(l bodyLine, served map[string]bool) []string {
	found := map[string]bool{}
	l.Text = loadSkillSpanRe().ReplaceAllString(l.Text, " ")
	for _, ref := range referenceNames(l) {
		if ref.kind == kindSkill && served[strings.ToLower(ref.name)] {
			found[ref.name] = true
		}
	}
	out := make([]string, 0, len(found))
	for name := range found {
		out = append(out, name)
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
				if namesTOMLString(l, src.Name) {
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

// Package cost reports which content items cost the most context: what each
// skill, agent, command, rule and context file adds to the prompt on every
// request (always loaded) and when it is opened (on demand), with the biggest
// offenders first and ceilings that turn a regression into an exit code.
//
// The per-runtime totals come from the generator's token report, the same
// numbers `ai-rulez tokens` prints; the per-item breakdown is estimated from the
// sources, so the two are reported side by side and never summed into each other.
package cost

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/tokens"
	"github.com/samber/oops"
)

// Item kinds.
const (
	KindRule    = "rule"
	KindContext = "context"
	KindSkill   = "skill"
	KindAgent   = "agent"
	KindCommand = "command"
)

// Item is the estimated cost of one content item.
type Item struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Domain string `json:"domain,omitempty"`
	// Always is paid on every request: a rule's or context file's body, or a
	// listed skill, agent or command's name and description plus framing.
	Always int `json:"always"`
	// Conditional is a path-scoped rule's body: paid only when matching files are in play.
	Conditional int `json:"conditional"`
	// OnDemand is a skill, agent or command body, paid when it is opened.
	OnDemand int `json:"on_demand"`
}

// Total is the item's cost if everything were loaded.
func (i Item) Total() int { return i.Always + i.Conditional + i.OnDemand }

// Limit is one checked ceiling.
type Limit struct {
	Limit    int  `json:"limit"`
	Actual   int  `json:"actual"`
	Exceeded bool `json:"exceeded"`
}

// Report is the cost view of one profile and target runtime.
type Report struct {
	Profile   string `json:"profile"`
	Target    string `json:"target"`
	Tokenizer string `json:"tokenizer"`
	// Runtime totals (authoritative, from the token report) for Target.
	Always      int `json:"always"`
	Conditional int `json:"conditional"`
	OnDemand    int `json:"on_demand"`
	// Items is every content item, most expensive first.
	Items []Item `json:"items"`
	// TopAlways and TopOnDemand are the biggest offenders of each kind of cost.
	TopAlways   []Item `json:"top_always"`
	TopOnDemand []Item `json:"top_on_demand"`
	// AlwaysBudget and OnDemandBudget are set when a ceiling was given.
	AlwaysBudget   *Limit   `json:"always_budget,omitempty"`
	OnDemandBudget *Limit   `json:"on_demand_budget,omitempty"`
	Notes          []string `json:"notes"`
}

// Exceeded reports whether any ceiling is over.
func (r *Report) Exceeded() bool {
	return (r.AlwaysBudget != nil && r.AlwaysBudget.Exceeded) || (r.OnDemandBudget != nil && r.OnDemandBudget.Exceeded)
}

// Options parameterizes Build.
type Options struct {
	Profile string
	// Target is the preset whose runtime totals to report; empty picks the
	// runtime with the largest always-loaded surface (the token report headline).
	Target string
	// Top bounds the offender lists; <= 0 means 10.
	Top     int
	Counter tokens.Counter
	// AlwaysBudget and OnDemandBudget are ceilings in tokens; 0 disables.
	AlwaysBudget, OnDemandBudget int
}

// Build computes the cost report.
func Build(cfg *config.Config, o Options) (*Report, error) {
	if o.Counter == nil {
		return nil, oops.Errorf("cost report requires a counter")
	}
	tr, err := generator.NewGenerator(cfg).TokenReport(generator.TokenReportOptions{Profile: o.Profile, Counter: o.Counter})
	if err != nil {
		return nil, err //nolint:wrapcheck // already contextual
	}
	runtime, err := pickRuntime(tr, o.Target)
	if err != nil {
		return nil, err
	}
	// A fresh generator above resolved the profile; the content selection below
	// uses the same resolved name.
	tree, err := cfg.GetContentForProfile(tr.Profile)
	if err != nil {
		return nil, oops.Wrapf(err, "select content for profile %q", tr.Profile)
	}
	rep := &Report{
		Profile: tr.Profile, Target: runtime.Preset, Tokenizer: tr.Tokenizer.Name,
		Always: runtime.Always, Conditional: runtime.Conditional, OnDemand: runtime.OnDemand,
		Items: itemsOf(tree, o.Counter),
		Notes: []string{
			"Runtime totals are the `ai-rulez tokens` figures for the target; the item table is estimated from the sources and does not add up to them exactly (file framing, root file sections and provider differences are not attributed to items).",
			"Counts are approximations (" + tr.Tokenizer.Name + "); the agent harness adds a fixed floor ai-rulez cannot see.",
		},
	}
	sort.SliceStable(rep.Items, func(i, j int) bool {
		if a, b := rep.Items[i].Total(), rep.Items[j].Total(); a != b {
			return a > b
		}
		return rep.Items[i].Name < rep.Items[j].Name
	})
	top := o.Top
	if top <= 0 {
		top = 10
	}
	rep.TopAlways = topBy(rep.Items, top, func(i Item) int { return i.Always })
	rep.TopOnDemand = topBy(rep.Items, top, func(i Item) int { return i.OnDemand })
	if o.AlwaysBudget > 0 {
		rep.AlwaysBudget = &Limit{Limit: o.AlwaysBudget, Actual: rep.Always, Exceeded: rep.Always > o.AlwaysBudget}
	}
	if o.OnDemandBudget > 0 {
		rep.OnDemandBudget = &Limit{Limit: o.OnDemandBudget, Actual: rep.OnDemand, Exceeded: rep.OnDemand > o.OnDemandBudget}
	}
	return rep, nil
}

func pickRuntime(tr *generator.TokenReport, target string) (generator.RuntimeTokens, error) {
	if len(tr.Runtimes) == 0 {
		return generator.RuntimeTokens{}, oops.Errorf("no generated output to measure; configure at least one preset")
	}
	want := target
	if want == "" {
		want = tr.HeadlinePreset
	}
	names := make([]string, 0, len(tr.Runtimes))
	for i := range tr.Runtimes {
		if tr.Runtimes[i].Preset == want {
			return tr.Runtimes[i], nil
		}
		names = append(names, tr.Runtimes[i].Preset)
	}
	return generator.RuntimeTokens{}, oops.Hint("Available targets: "+strings.Join(names, ", ")).Errorf("unknown target %q", target)
}

func topBy(items []Item, n int, by func(Item) int) []Item {
	cp := make([]Item, 0, len(items))
	for _, it := range items {
		if by(it) > 0 {
			cp = append(cp, it)
		}
	}
	sort.SliceStable(cp, func(i, j int) bool { return by(cp[i]) > by(cp[j]) })
	if len(cp) > n {
		cp = cp[:n]
	}
	return cp
}

func itemsOf(tree *config.ContentTree, c tokens.Counter) []Item {
	var out []Item
	add := func(domain string, t *config.ContentTree) {
		for _, f := range t.Rules {
			out = append(out, bodyItem(KindRule, domain, f, c))
		}
		for _, f := range t.Context {
			out = append(out, bodyItem(KindContext, domain, f, c))
		}
		for _, f := range t.Skills {
			out = append(out, listedItem(KindSkill, domain, f, c))
		}
		for _, f := range t.Agents {
			out = append(out, listedItem(KindAgent, domain, f, c))
		}
		for _, f := range t.Commands {
			out = append(out, listedItem(KindCommand, domain, f, c))
		}
	}
	add("", tree)
	names := make([]string, 0, len(tree.Domains))
	for n := range tree.Domains {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		if d := tree.Domains[n]; d != nil {
			add(n, &config.ContentTree{Rules: d.Rules, Context: d.Context, Skills: d.Skills, Agents: d.Agents, Commands: d.Commands})
		}
	}
	return out
}

func itemName(f config.ContentFile) string {
	base := f.Name
	if base == "" {
		base = strings.TrimSuffix(filepath.Base(f.Path), filepath.Ext(f.Path))
	}
	if strings.EqualFold(base, "SKILL") || strings.EqualFold(base, "COMMAND") {
		if dir := filepath.Base(filepath.Dir(f.Path)); dir != "." && dir != string(filepath.Separator) {
			return dir
		}
	}
	return base
}

// bodyItem is a rule or context file: its body rides in the prompt, always or
// only for matching paths.
func bodyItem(kind, domain string, f config.ContentFile, c tokens.Counter) Item {
	n := c.Count(f.Content)
	it := Item{Kind: kind, Name: itemName(f), Domain: domain}
	if f.Metadata != nil && len(f.Metadata.PathScope()) > 0 {
		it.Conditional = n
	} else {
		it.Always = n
	}
	return it
}

// listedItem is a skill, agent or command: the harness lists its name and
// description each request (unless the model may not invoke it) and reads the
// body when it is opened.
func listedItem(kind, domain string, f config.ContentFile, c tokens.Counter) Item {
	name := itemName(f)
	it := Item{Kind: kind, Name: name, Domain: domain, OnDemand: c.Count(f.Content)}
	if f.Metadata != nil && strings.EqualFold(strings.TrimSpace(f.Metadata.Extra["disable-model-invocation"]), "true") {
		return it
	}
	desc := ""
	if f.Metadata != nil {
		desc = config.SkillDescription(f.Metadata)
	}
	it.Always = c.Count(name+" "+desc) + generator.ListingEntryOverheadTokens
	return it
}

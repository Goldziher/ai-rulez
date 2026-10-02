package generator

// Token accounting for generated artifacts.
//
// This lives inside the generator package on purpose. The number a report has to
// show is the cost of the bytes an agent actually loads, which is not what
// collectOutputs returns: writeOutput additionally applies stripHeader,
// HashContent, injectHashes and normalizeTrailingNewline before anything reaches
// disk, and all four are package-private. Exporting collectOutputs alone would
// force every caller to re-implement that pipeline and drift from writeOutput
// the first time it changes, so the seam we export is the finished report.
//
// Nothing here reads a generated file back off disk. A stale or half-written
// tree would silently corrupt every number, and the tree need not exist at all
// for the report to be correct.

import (
	"cmp"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/tokens"
	"github.com/samber/oops"
)

// Bucket says when an agent pays for a slice of generated output.
type Bucket string

// Buckets, ordered from "paid on every request" to "ai-rulez cannot say".
const (
	// BucketAlways is surface in the prompt on every request of every session.
	BucketAlways Bucket = "always"
	// BucketConditional is surface some harness modes include and others do not.
	// Skill and command descriptions are the measured case: a non-interactive
	// `claude -p` session does not carry them, an interactive session surfaces
	// them for user-invocable skills.
	BucketConditional Bucket = "conditional"
	// BucketOnDemand is surface an agent reads only when it opens the artifact.
	BucketOnDemand Bucket = "on_demand"
	// BucketUnmodeled is surface ai-rulez generates but whose loading rule it
	// does not know for the runtime in question. Never folded into a total that
	// claims to be always-loaded.
	BucketUnmodeled Bucket = "unmodeled"
)

// RootRuntimeScope is the Scope value of outputs written at the repository root
// rather than into a configured scope subdirectory.
const RootRuntimeScope = ""

// rootDomainLabel names content that lives outside any domain.
const rootDomainLabel = "(root)"

// TokenizerInfo describes how a report's numbers were produced.
type TokenizerInfo struct {
	Name string `json:"name"`
	// Approximate is always true. Claude's tokenizer is not published, so no
	// counter available offline can be exact for it.
	Approximate bool `json:"approximate"`
	// Estimate is true when Name refers to a bytes-per-token ratio rather than a
	// byte-pair encoder.
	Estimate bool `json:"estimate"`
}

// Entry is one line of a runtime's breakdown.
type Entry struct {
	Label  string `json:"label"`
	Bucket Bucket `json:"bucket"`
	Tokens int    `json:"tokens"`
	// Artifacts is how many generated artifacts this line folds together. 1 for
	// a single file or section.
	Artifacts int     `json:"artifacts"`
	Children  []Entry `json:"children,omitempty"`
}

// RuntimeTokens is the cost of everything ai-rulez generates for one provider.
//
// Totals are never additive across runtimes: a session runs one agent, which
// reads one runtime's root file. A repository that emits both CLAUDE.md and
// AGENTS.md pays for one of them per session, not both.
type RuntimeTokens struct {
	Preset string `json:"preset"`
	// Scope is the scope subdirectory these outputs belong to, or "" for the
	// repository root.
	Scope       string `json:"scope,omitempty"`
	Files       int    `json:"files"`
	Always      int    `json:"always"`
	Conditional int    `json:"conditional"`
	OnDemand    int    `json:"on_demand"`
	Unmodeled   int    `json:"unmodeled"`
	// Detailed is true when the provider is described by the provider DSL, whose
	// renderer reports an exact artifact kind and a per-section split of the root
	// instructions file. False for the hand-written preset generators, whose
	// outputs are classified from their paths and whose root file is measured as
	// one undifferentiated total.
	Detailed bool `json:"detailed"`
	// RootFiles counts the instructions files in this runtime. A provider that
	// emits only a sidecar (mcp) has none, so Detailed being false says nothing
	// about it and must not raise a caveat.
	RootFiles int     `json:"root_files"`
	Entries   []Entry `json:"entries"`
}

// DomainTokens is one row of the per-domain total.
type DomainTokens struct {
	Name        string `json:"name"`
	Always      int    `json:"always"`
	Conditional int    `json:"conditional"`
	OnDemand    int    `json:"on_demand"`
}

// BudgetResult is the outcome of a ceiling check against the headline figure.
type BudgetResult struct {
	Limit    int  `json:"limit"`
	Actual   int  `json:"actual"`
	Exceeded bool `json:"exceeded"`
}

// TokenReport is the always-loaded and on-demand token surface of one profile.
type TokenReport struct {
	Profile   string        `json:"profile"`
	Tokenizer TokenizerInfo `json:"tokenizer"`
	// HeadlinePreset is the root-scope runtime with the largest always-loaded
	// surface, and HeadlineAlways is that surface. It is the figure to watch and
	// the figure a budget is checked against, because it is the worst case a
	// single session can pay for the artifacts ai-rulez controls.
	HeadlinePreset string          `json:"headline_preset"`
	HeadlineAlways int             `json:"headline_always"`
	Runtimes       []RuntimeTokens `json:"runtimes"`
	Scoped         []RuntimeTokens `json:"scoped,omitempty"`
	Domains        []DomainTokens  `json:"domains"`
	Budget         *BudgetResult   `json:"budget,omitempty"`
	Notes          []string        `json:"notes"`
}

// TokenReportOptions parameterises TokenReport.
type TokenReportOptions struct {
	// Profile is the profile to render. Empty resolves the same way generate
	// resolves it.
	Profile string
	// Counter counts tokens. Required.
	Counter tokens.Counter
	// Budget is the always-loaded ceiling for the headline runtime. Zero or less
	// disables the check.
	Budget int
}

// TokenReport renders the configuration in memory and reports the token surface
// of the result, split by when an agent loads it.
//
// The generator must not be reused across profiles: collectOutputs writes
// g.config.SourceHash, so build a fresh Generator per profile.
func (g *Generator) TokenReport(options TokenReportOptions) (*TokenReport, error) {
	if options.Counter == nil {
		return nil, oops.Errorf("token report requires a counter")
	}

	collector := config.NewAnalysisCollector()
	g.config.Analysis = collector
	defer func() { g.config.Analysis = nil }()

	outputs, activeProfile, err := g.collectOutputs(options.Profile)
	if err != nil {
		return nil, err
	}

	contentTree, err := g.getContentForProfile(activeProfile)
	if err != nil {
		return nil, err
	}

	builder := &reportBuilder{
		counter:      options.Counter,
		finalPayload: g.finalPayloadByPath(outputs),
		domainByPath: domainIndex(contentTree),
	}

	report := &TokenReport{
		Profile: activeProfile,
		Tokenizer: TokenizerInfo{
			Name:        options.Counter.Name(),
			Approximate: true,
			Estimate:    options.Counter.IsEstimate(),
		},
	}
	builder.build(collector.Analyses(), report)
	report.Notes = reportNotes(report)
	if options.Budget > 0 {
		report.Budget = &BudgetResult{
			Limit:    options.Budget,
			Actual:   report.HeadlineAlways,
			Exceeded: report.HeadlineAlways > options.Budget,
		}
	}
	return report, nil
}

// finalPayloadByPath maps every non-directory output path to the exact string
// writeOutput would put on disk. That is the payload an agent loads: the
// rendered content plus the injected Content-Hash and Source-Hash provenance
// lines, normalized to a single trailing newline.
func (g *Generator) finalPayloadByPath(outputs []config.OutputFile) map[string]string {
	result := make(map[string]string, len(outputs))
	for _, output := range outputs {
		if output.IsDir {
			continue
		}
		if output.RawContent != nil {
			// Raw outputs (skill scripts, binary assets) are written verbatim.
			result[output.Path] = string(output.RawContent)
			continue
		}
		result[output.Path] = g.finalContent(output)
	}
	return result
}

// domainIndex maps every authored content path to the domain that owns it.
func domainIndex(content *config.ContentTree) map[string]string {
	index := make(map[string]string)
	if content == nil {
		return index
	}
	record := func(domain string, files ...[]config.ContentFile) {
		for _, slice := range files {
			for _, file := range slice {
				if file.Path != "" {
					index[file.Path] = domain
				}
			}
		}
	}
	record(rootDomainLabel, content.Rules, content.Context, content.Skills, content.Agents, content.Commands)
	for name, domain := range content.Domains {
		if domain == nil {
			continue
		}
		record(name, domain.Rules, domain.Context, domain.Skills, domain.Agents, domain.Commands)
	}
	return index
}

// reportBuilder accumulates a report from recorded analyses.
type reportBuilder struct {
	counter      tokens.Counter
	finalPayload map[string]string
	domainByPath map[string]string
	// domains is keyed by runtime first, then by domain. Domain cost is tallied
	// per runtime and reported for the headline one only: every runtime renders the
	// same authored rules, so a single tally summed across all of them would
	// multiply each domain by the number of providers and read far above the
	// headline it is meant to explain.
	domains        map[string]map[string]*DomainTokens
	currentRuntime string
}

func (b *reportBuilder) build(analyses []*config.OutputAnalysis, report *TokenReport) {
	b.domains = make(map[string]map[string]*DomainTokens)

	groups := make(map[string]*runtimeGroup)
	var order []string
	for _, analysis := range analyses {
		key := runtimeKey(analysis.Preset, analysis.Scope)
		group, ok := groups[key]
		if !ok {
			group = &runtimeGroup{preset: analysis.Preset, scope: analysis.Scope}
			groups[key] = group
			order = append(order, key)
		}
		group.add(analysis)
	}

	for _, key := range order {
		b.currentRuntime = key
		runtime := b.buildRuntime(groups[key])
		if runtime.Scope == RootRuntimeScope {
			report.Runtimes = append(report.Runtimes, runtime)
		} else {
			report.Scoped = append(report.Scoped, runtime)
		}
	}
	b.currentRuntime = ""

	sortRuntimes(report.Runtimes)
	sortRuntimes(report.Scoped)
	if len(report.Runtimes) > 0 {
		report.HeadlinePreset = report.Runtimes[0].Preset
		report.HeadlineAlways = report.Runtimes[0].Always
		report.Domains = b.domainRows(runtimeKey(report.Runtimes[0].Preset, report.Runtimes[0].Scope))
	}
}

// runtimeKey identifies one provider's output in one scope.
func runtimeKey(preset, scope string) string { return preset + "\x00" + scope }

func sortRuntimes(runtimes []RuntimeTokens) {
	slices.SortStableFunc(runtimes, func(left, right RuntimeTokens) int {
		if diff := cmp.Compare(right.Always, left.Always); diff != 0 {
			return diff
		}
		return cmp.Compare(left.Preset, right.Preset)
	})
}

// runtimeGroup is the set of analyses one provider produced in one scope.
type runtimeGroup struct {
	preset   string
	scope    string
	roots    []*config.OutputAnalysis
	ruleFile []*config.OutputAnalysis
	items    map[config.OutputKind][]*config.OutputAnalysis
	other    []*config.OutputAnalysis
	// files counts distinct outputs. Entry.Artifacts cannot be summed for this:
	// one file contributes to several entries (its name, its description and its
	// body are separate lines), so summing them counts most files three times.
	files int
}

func (g *runtimeGroup) add(analysis *config.OutputAnalysis) {
	g.files++
	switch analysis.Kind {
	case config.OutputKindRoot, config.OutputKindLocalRoot:
		g.roots = append(g.roots, analysis)
	case config.OutputKindRuleFile:
		g.ruleFile = append(g.ruleFile, analysis)
	case config.OutputKindSkill, config.OutputKindAgent, config.OutputKindCommand:
		if g.items == nil {
			g.items = make(map[config.OutputKind][]*config.OutputAnalysis)
		}
		g.items[analysis.Kind] = append(g.items[analysis.Kind], analysis)
	default:
		g.other = append(g.other, analysis)
	}
}

func (b *reportBuilder) buildRuntime(group *runtimeGroup) RuntimeTokens {
	runtime := RuntimeTokens{
		Preset:    group.preset,
		Scope:     group.scope,
		Files:     group.files,
		RootFiles: len(group.roots),
	}

	for _, root := range group.roots {
		runtime.Entries = append(runtime.Entries, b.rootEntry(root))
		runtime.Detailed = runtime.Detailed || hasSectionDetail(root)
	}

	if len(group.ruleFile) > 0 {
		runtime.Entries = append(runtime.Entries, b.aggregate(
			"provider rule files", BucketAlways, group.ruleFile, allPartKinds))
	}

	for _, kind := range []config.OutputKind{config.OutputKindSkill, config.OutputKindCommand, config.OutputKindAgent} {
		runtime.Entries = append(runtime.Entries, b.itemEntries(kind, group.items[kind])...)
	}

	if len(group.other) > 0 {
		runtime.Entries = append(runtime.Entries, b.otherEntries(group.other)...)
	}

	provenance := b.provenanceEntry(group)
	if provenance.Tokens > 0 {
		runtime.Entries = append(runtime.Entries, provenance)
	}

	for _, entry := range runtime.Entries {
		switch entry.Bucket {
		case BucketAlways:
			runtime.Always += entry.Tokens
		case BucketConditional:
			runtime.Conditional += entry.Tokens
		case BucketOnDemand:
			runtime.OnDemand += entry.Tokens
		case BucketUnmodeled:
			runtime.Unmodeled += entry.Tokens
		}
	}
	return runtime
}

// hasSectionDetail reports whether an analysis carries a real per-section split
// rather than one whole-file part.
func hasSectionDetail(analysis *config.OutputAnalysis) bool {
	for _, part := range analysis.Parts {
		switch part.Kind {
		case config.PartKindRootTitle, config.PartKindRootDescription,
			config.PartKindRootRule, config.PartKindRootContext,
			config.PartKindRootAgentsRoster:
			return true
		}
	}
	return false
}

// rootEntry measures a root instructions file: the whole file as the entry's
// total, its recorded sections as children, and the difference as a residual.
func (b *reportBuilder) rootEntry(analysis *config.OutputAnalysis) Entry {
	bucket := BucketAlways
	label := relativeLabel(analysis.Path)
	if analysis.Kind == config.OutputKindLocalRoot {
		// A machine-local root is gitignored and present only on the developer's
		// own machine, so it is not part of the surface a repository ships.
		bucket = BucketConditional
		label += " (machine-local)"
	}

	entry := Entry{Label: label, Bucket: bucket, Artifacts: 1}
	entry.Tokens = b.counter.Count(b.finalPayload[analysis.Path])

	// Children keep the order the provider DSL renders its sections in, so the
	// report reads top-to-bottom like the file it describes. Rules and context
	// collapse into one parent each, inserted where their first entry appeared.
	sectioned := 0
	groupIndex := map[config.PartKind]int{}
	for _, part := range analysis.Parts {
		count := b.counter.Count(part.Content)
		sectioned += count
		b.attribute(part.SourcePath, bucket, count)
		child := Entry{Label: string(part.Kind), Bucket: bucket, Tokens: count, Artifacts: 1}

		groupLabel := ""
		switch part.Kind {
		case config.PartKindRootRule:
			groupLabel = "rules_inline"
		case config.PartKindRootContext:
			groupLabel = "context_inline"
		}
		if groupLabel == "" {
			entry.Children = append(entry.Children, child)
			continue
		}

		child.Label = part.Label
		index, ok := groupIndex[part.Kind]
		if !ok {
			index = len(entry.Children)
			groupIndex[part.Kind] = index
			entry.Children = append(entry.Children, Entry{Label: groupLabel, Bucket: bucket})
		}
		parent := &entry.Children[index]
		parent.Tokens += count
		parent.Artifacts++
		parent.Children = append(parent.Children, child)
	}

	if residual := entry.Tokens - sectioned; residual != 0 && sectioned > 0 {
		// Provenance hashes are injected after the sections are rendered, and a
		// byte-pair encoder does not tokenize a concatenation as the sum of its
		// pieces. Both differences land here rather than being hidden.
		entry.Children = append(entry.Children, Entry{
			Label:     "provenance hashes and section boundaries",
			Bucket:    bucket,
			Tokens:    residual,
			Artifacts: 1,
		})
	}
	return entry
}

// itemBuckets is the loading model for a per-item artifact, by part kind.
//
// Calibrated against Claude Code. Ablating one component at a time from a real
// 198-skill, 32-agent tree showed that a skill costs about four prompt tokens —
// its name in the skill listing — and nothing else: inflating all 198
// descriptions by roughly 17,000 tokens of text moved the measured prompt by 19
// tokens, and a session asked to quote a description verbatim could not. Agents
// are different: the harness injects each agent's name and description into the
// Agent tool schema, which measured about 48 tokens per agent for 32 agents.
type itemModel struct {
	nameBucket        Bucket
	descriptionBucket Bucket
}

func modelFor(kind config.OutputKind) itemModel {
	switch kind {
	case config.OutputKindAgent:
		return itemModel{nameBucket: BucketAlways, descriptionBucket: BucketAlways}
	default:
		// Skills and commands: the name reaches the prompt, the description only
		// in harness modes that list user-invocable skills.
		return itemModel{nameBucket: BucketAlways, descriptionBucket: BucketConditional}
	}
}

var overheadPartKinds = []config.PartKind{
	config.PartKindItemFrontmatter,
	config.PartKindItemResourceIndex,
	config.PartKindItemTargetedRules,
	config.PartKindItemTargetedContext,
}

// allPartKinds selects every recorded part, used for artifacts with no field
// split (a provider rule file, a whole-file fallback).
var allPartKinds []config.PartKind

func (b *reportBuilder) itemEntries(kind config.OutputKind, analyses []*config.OutputAnalysis) []Entry {
	if len(analyses) == 0 {
		return nil
	}
	model := modelFor(kind)
	noun := string(kind)
	entries := []Entry{
		b.aggregate(noun+" names", model.nameBucket, analyses, []config.PartKind{config.PartKindItemName}),
		b.aggregate(noun+" descriptions", model.descriptionBucket, analyses, []config.PartKind{config.PartKindItemDescription}),
		b.aggregate(noun+" bodies", BucketOnDemand, analyses, []config.PartKind{config.PartKindItemBody}),
		b.aggregate(noun+" file overhead", BucketOnDemand, analyses, overheadPartKinds),
	}
	result := make([]Entry, 0, len(entries))
	for _, entry := range entries {
		if entry.Tokens > 0 {
			result = append(result, entry)
		}
	}
	return result
}

func (b *reportBuilder) otherEntries(analyses []*config.OutputAnalysis) []Entry {
	byKind := make(map[config.OutputKind][]*config.OutputAnalysis)
	var kinds []config.OutputKind
	for _, analysis := range analyses {
		if _, ok := byKind[analysis.Kind]; !ok {
			kinds = append(kinds, analysis.Kind)
		}
		byKind[analysis.Kind] = append(byKind[analysis.Kind], analysis)
	}
	slices.Sort(kinds)

	var result []Entry
	for _, kind := range kinds {
		label, bucket := otherLabel(kind)
		entry := b.aggregate(label, bucket, byKind[kind], allPartKinds)
		if entry.Tokens > 0 || entry.Artifacts > 0 {
			result = append(result, entry)
		}
	}
	return result
}

func otherLabel(kind config.OutputKind) (label string, bucket Bucket) {
	switch kind {
	case config.OutputKindResource:
		return "skill resources", BucketOnDemand
	case config.OutputKindSidecar:
		// A settings or MCP manifest is configuration the harness consumes, not
		// text it pastes into the prompt — but an MCP server declared there does
		// produce tool schemas whose size ai-rulez cannot see.
		return "sidecar documents", BucketUnmodeled
	default:
		return "unclassified outputs", BucketUnmodeled
	}
}

// aggregate folds one part kind (or all of them) across many artifacts into a
// single line, and attributes each artifact's tokens to its domain.
func (b *reportBuilder) aggregate(label string, bucket Bucket, analyses []*config.OutputAnalysis, kinds []config.PartKind) Entry {
	entry := Entry{Label: label, Bucket: bucket}
	for _, analysis := range analyses {
		matched := false
		for _, part := range analysis.Parts {
			if kinds != nil && !slices.Contains(kinds, part.Kind) {
				continue
			}
			count := b.counter.Count(part.Content)
			entry.Tokens += count
			b.attribute(partSource(analysis, part), bucket, count)
			matched = true
		}
		if matched {
			entry.Artifacts++
		}
	}
	return entry
}

// provenanceEntry accounts for the bytes writeOutput adds to every artifact that
// is not a root file: the Content-Hash and Source-Hash lines, plus trailing
// newline normalization. Reported as one line so no token in a generated tree is
// silently unaccounted for.
//
// This is not a rounding error. A blake3 hex digest is 64 characters of
// incompressible hex, which cl100k splits into roughly 32 tokens; the two lines
// together measure 94 tokens standalone and add about 76 per artifact in context.
// A tree with 200 skills therefore pays five figures of on-demand surface for
// provenance alone, which is exactly the kind of cost a per-file total hides.
func (b *reportBuilder) provenanceEntry(group *runtimeGroup) Entry {
	entry := Entry{Label: "per-file provenance hashes", Bucket: BucketOnDemand}
	var analyses []*config.OutputAnalysis
	analyses = append(analyses, group.ruleFile...)
	for _, kind := range []config.OutputKind{config.OutputKindSkill, config.OutputKindCommand, config.OutputKindAgent} {
		analyses = append(analyses, group.items[kind]...)
	}
	analyses = append(analyses, group.other...)

	for _, analysis := range analyses {
		rendered := 0
		for _, part := range analysis.Parts {
			rendered += b.counter.Count(part.Content)
		}
		final := b.counter.Count(b.finalPayload[analysis.Path])
		if delta := final - rendered; delta > 0 {
			entry.Tokens += delta
			entry.Artifacts++
		}
	}
	return entry
}

func partSource(analysis *config.OutputAnalysis, part config.OutputPart) string {
	if part.SourcePath != "" {
		return part.SourcePath
	}
	return analysis.SourcePath
}

func (b *reportBuilder) attribute(sourcePath string, bucket Bucket, count int) {
	if count == 0 {
		return
	}
	name, ok := b.domainByPath[sourcePath]
	if !ok {
		name = rootDomainLabel
	}
	rows, ok := b.domains[b.currentRuntime]
	if !ok {
		rows = make(map[string]*DomainTokens)
		b.domains[b.currentRuntime] = rows
	}
	row, ok := rows[name]
	if !ok {
		row = &DomainTokens{Name: name}
		rows[name] = row
	}
	switch bucket {
	case BucketAlways:
		row.Always += count
	case BucketConditional:
		row.Conditional += count
	case BucketOnDemand:
		row.OnDemand += count
	}
}

func (b *reportBuilder) domainRows(runtime string) []DomainTokens {
	tallied := b.domains[runtime]
	rows := make([]DomainTokens, 0, len(tallied))
	for _, row := range tallied {
		rows = append(rows, *row)
	}
	sort.Slice(rows, func(left, right int) bool {
		if rows[left].Always != rows[right].Always {
			return rows[left].Always > rows[right].Always
		}
		return rows[left].Name < rows[right].Name
	})
	return rows
}

// relativeLabel shortens an absolute output path to its last two segments, which
// is enough to identify it without printing a machine-specific prefix.
func relativeLabel(path string) string {
	slashed := filepath.ToSlash(path)
	segments := strings.Split(slashed, "/")
	if len(segments) <= 2 {
		return slashed
	}
	return strings.Join(segments[len(segments)-1:], "/")
}

// reportNotes states what the numbers are and, more importantly, what they are
// not. Requirement, not decoration: a bare token count read as a session
// prediction is worse than no number at all.
func reportNotes(report *TokenReport) []string {
	notes := []string{
		"Counts are approximations. Claude's tokenizer is not published; this report uses " +
			report.Tokenizer.Name + " offline. Use the numbers to compare profiles and to " +
			"compare before and after an edit, not as absolute truth.",
		"ai-rulez counts only the artifacts it generates. The agent harness adds a fixed " +
			"floor of its own system prompt and tool schemas, plus per-artifact overhead, " +
			"neither of which ai-rulez can see. This report never predicts a session total.",
		"Runtimes are not additive. One session loads one runtime's root instructions file, " +
			"so emitting both CLAUDE.md and AGENTS.md costs one of them, not both. The " +
			"headline figure is the largest single runtime.",
		"\"conditional\" is surface some harness modes carry and others do not. Skill and " +
			"command descriptions are the measured case: a non-interactive Claude Code run " +
			"does not carry them, an interactive session surfaces them for user-invocable skills.",
		"Provenance hash lines are content-dependent: a blake3 hex digest is incompressible, " +
			"and two digests of the same length tokenize to slightly different counts. Expect a " +
			"few tokens of movement per artifact between two profiles for that reason alone.",
	}
	if report.Tokenizer.Estimate {
		notes = append(notes, "This run used a bytes-per-token estimate, not a tokenizer. "+
			"The ratio holds for a single prose artifact and has been measured between 1.81 "+
			"and 5.30 bytes per token across whole trees, so a tree-level total from it can be "+
			"wrong by a factor of three.")
	}
	for _, runtime := range append(append([]RuntimeTokens{}, report.Runtimes...), report.Scoped...) {
		if runtime.RootFiles > 0 && !runtime.Detailed {
			notes = append(notes, "Preset \""+runtime.Preset+"\" is not described by the provider "+
				"DSL, so its outputs are classified from their paths and its instructions file is "+
				"measured as one total with no per-section split.")
		}
	}
	if len(report.Scoped) > 0 {
		notes = append(notes, "Scoped outputs are listed separately: an agent loads them only "+
			"while working inside that subdirectory, so they are not part of the headline.")
	}
	return notes
}

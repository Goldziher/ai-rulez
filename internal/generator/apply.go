package generator

import (
	"fmt"
	"sort"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/diag"
	"github.com/Goldziher/ai-rulez/v5/internal/templates"
)

// RunPlan is what a generate run renders before it acts on the project: the
// profile that was resolved and every file and directory the presets produce, in
// write order. Rendering reads the project (the previous manifest, merged
// documents, hand-written rule files) but writes nothing; an Applier then writes
// the plan, previews it, compares it with the disk or describes it.
//
// A RunPlan belongs to the Generator that planned it: the rendering leaves the
// bookkeeping the appliers read (the manifests read, the claims merged
// documents carry) on the Generator.
type RunPlan struct {
	// Requested is the profile asked for; "" selects the configured default.
	Requested string
	// Profile is the profile that was rendered.
	Profile string
	// Outputs are the files and directories the run produces, in write order.
	Outputs []config.OutputFile

	owner *Generator
}

// ApplyResult is what an Applier reports; which fields are set depends on the Applier.
type ApplyResult struct {
	// Written is the number of files DiskApplier wrote, directories excluded.
	Written int
	// Lines are the plan DryRunApplier lists, one action per line.
	Lines []string
	// Drift are the files CheckApplier found to differ from the plan.
	Drift []Drift
	// Document is the plan DescribeApplier reports (see Plan).
	Document *Plan
}

// Applier turns a RunPlan into an effect. The four implementations are
// DiskApplier (write the project), DryRunApplier (list what writing would do),
// CheckApplier (report drift without writing) and DescribeApplier (the plan
// document behind `generate --emit-plan`). The interface is sealed: a method that
// is not exported keeps another package from implementing it, so it can grow
// without breaking anyone.
type Applier interface {
	apply(g *Generator, p *RunPlan) (*ApplyResult, error)
	// lifecycle says how the run around the apply is bracketed.
	lifecycle() runLifecycle
}

// runLifecycle is the per-run bookkeeping around an apply.
type runLifecycle struct {
	// downgrades collects the rule-file downgrade warnings of the run and issues
	// them when it ends.
	downgrades bool
	// quiet resets the diagnostics when the run starts and ends without issuing
	// the downgrade summary: a run that only compares (--check) stays silent and
	// leaves no Once keys behind for the next run on the same Generator.
	quiet bool
	// forgetState drops the state a run leaves on the Generator once it ends.
	forgetState bool
}

// The appliers.
var (
	// DiskApplier writes the plan: stale files removed, outputs written, merged
	// documents unmerged, manifests and .gitignore updated.
	DiskApplier Applier = diskApplier{}
	// DryRunApplier lists what DiskApplier would do and writes nothing.
	DryRunApplier Applier = dryRunApplier{}
	// CheckApplier compares the plan with the disk and writes nothing.
	CheckApplier Applier = checkApplier{}
	// DescribeApplier reports the plan as a document (files with digests, removals),
	// without header stamps and resolved environment values; nothing is written.
	DescribeApplier Applier = describeApplier{}
)

// Plan renders the outputs for profile ("" is the configured default) and
// returns them without writing anything.
func (g *Generator) Plan(profile string) (*RunPlan, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.beginRun()
	d := g.diagnostics()
	d.Reset()
	defer d.Flush()
	return g.render(profile)
}

// Apply applies p, which this Generator planned, with a. The state a plan leaves
// on the Generator is consumed by one Apply: plan again to apply again.
func (g *Generator) Apply(p *RunPlan, a Applier) (*ApplyResult, error) {
	if p == nil || p.owner != g {
		return nil, oops.Errorf("apply: the plan was not made by this generator")
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	lc := a.lifecycle()
	if lc.downgrades {
		d := g.diagnostics()
		d.Reset()
		defer d.Flush()
	}
	if lc.quiet {
		d := g.diagnostics()
		d.Reset()
		defer d.Reset()
	}
	if lc.forgetState {
		defer g.resetRunState()
	}
	return a.apply(g, p)
}

// run is Plan and Apply as one serialized run, which is how generate, --dry-run,
// --check and --emit-plan execute.
func (g *Generator) run(profile string, a Applier) (*ApplyResult, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.beginRun()
	lc := a.lifecycle()
	if lc.downgrades {
		d := g.diagnostics()
		d.Reset()
		defer d.Flush()
	}
	if lc.quiet {
		d := g.diagnostics()
		d.Reset()
		defer d.Reset()
	}
	if lc.forgetState {
		defer g.resetRunState()
	}
	p, err := g.render(profile)
	if err != nil {
		return nil, err
	}
	return a.apply(g, p)
}

// render renders the outputs of a plan. The caller holds g.mu.
func (g *Generator) render(profile string) (*RunPlan, error) {
	outputs, active, err := g.collectOutputs(profile)
	if err != nil {
		return nil, err
	}
	return &RunPlan{Requested: profile, Profile: active, Outputs: outputs, owner: g}, nil
}

type diskApplier struct{}

func (diskApplier) lifecycle() runLifecycle { return runLifecycle{downgrades: true} }

// apply is the write half of generate. Order matters: machine-local and
// secret-carrying outputs are git-ignored before they are written, and the run
// can be refused before any write.
func (diskApplier) apply(g *Generator, p *RunPlan) (*ApplyResult, error) {
	flatOutputs := p.Outputs

	// Nothing is written, ignored or removed when a file ai-rulez cannot prove it
	// wrote (or a symlinked output) is in the way: the user decides first.
	if err := g.checkOutputSafety(flatOutputs); err != nil {
		return nil, err
	}

	// The machine-local inputs (overlay, local/ tree) are ignored before any check
	// can refuse the run, so a refused first run never leaves them unignored.
	if err := g.ignoreLocalInputs(); err != nil {
		return nil, err
	}

	if err := g.guardLocal(p.Requested, flatOutputs); err != nil {
		return nil, err
	}

	if role := g.Role(); role != "" {
		// A role renders its own slice of the content, not a profile.
		g.log().Info("Generating with configuration", "role", role)
	} else {
		g.log().Info("Generating with configuration", "profile", p.Profile)
	}
	if g.config.HasGuard() {
		g.log().Info("Generated-file guard: only harnesses with a blocking PreToolUse hook get it; the others are skipped",
			"harnesses", config.GuardHarnesses)
	}

	if err := g.ensureSecretOutputsIgnored(flatOutputs); err != nil {
		return nil, err
	}
	g.markSensitiveOutputs(flatOutputs)
	g.planLocalManifest(flatOutputs)

	ignoredEarly, err := g.ignoreBeforeWriting(flatOutputs)
	if err != nil {
		return nil, err
	}

	// Stale files go only once every output is written: a write that fails
	// leaves the previous generation whole instead of half removed.
	staleFiles := g.retiredFiles(flatOutputs)
	if err := g.writeOutputs(flatOutputs); err != nil {
		return nil, err
	}
	g.removeStaleManifestFiles(staleFiles)

	// Merged documents lose what an earlier run merged in and this one does not
	// (a preset or server that was removed). Planned after the write so that
	// what this run claimed counts.
	unmerged := g.planUnmerge(flatOutputs, false)
	g.applyUnmerge(unmerged)

	// Writing happens first: a directory the stale pass emptied may be one this
	// run re-creates, and pruning before the write would only have it made again.
	g.pruneDirsEmptiedBy(append(staleFiles, deletedPaths(unmerged)...))

	if err := g.writeGeneratedManifest(flatOutputs); err != nil {
		if g.hasLocalOutputs(flatOutputs) {
			// Without the local manifest a later run cannot clean these files up.
			return nil, oops.Wrapf(err, "write the generated manifests")
		}
		g.log().Warn("Failed to write generated manifest", "error", err)
	}

	g.finishGitignore(flatOutputs, ignoredEarly)

	written := 0
	for _, output := range flatOutputs {
		if !output.IsDir {
			written++
		}
	}

	g.log().Info("Generation complete", "files", written)

	return &ApplyResult{Written: written}, nil
}

type dryRunApplier struct{}

func (dryRunApplier) lifecycle() runLifecycle { return runLifecycle{downgrades: true} }

func (dryRunApplier) apply(g *Generator, p *RunPlan) (*ApplyResult, error) {
	flatOutputs := p.Outputs
	g.refusedOutputs, g.linkedOutputs = g.outputSafety(flatOutputs)
	local, err := g.planLocal(p.Requested, flatOutputs)
	if err != nil {
		return nil, err
	}

	lines := []string{fmt.Sprintf("profile: %s", p.Profile)}
	if local != nil {
		lines = append(lines, local.dryRunLines()...)
	}
	lines = append(lines, g.planLines(flatOutputs)...)
	for _, stale := range g.retiredFiles(flatOutputs) {
		lines = append(lines, "delete-stale: "+g.convertToRelativePath(stale))
	}
	for _, edit := range g.planUnmerge(flatOutputs, false) {
		if edit.delete {
			lines = append(lines, "delete-stale: "+edit.rel)
		} else {
			lines = append(lines, "unmerge: "+edit.rel)
		}
	}
	return &ApplyResult{Lines: lines}, nil
}

type checkApplier struct{}

func (checkApplier) lifecycle() runLifecycle { return runLifecycle{quiet: true, forgetState: true} }

func (checkApplier) apply(g *Generator, p *RunPlan) (*ApplyResult, error) {
	outputs := p.Outputs
	g.previousFiles = nil
	// Classify the machine-local inputs like generate does: the outputs get the
	// Source-Hash and local flags generate would write, so files that are in sync
	// are not reported, and what generate would refuse to write is reported as blocked.
	local, err := g.planLocal(p.Requested, outputs)
	if err != nil {
		return nil, err
	}
	blocked := map[string]bool{}
	if local != nil && !g.allowLocalDrift {
		for _, v := range local.violations {
			blocked[v.path] = true
		}
	}
	var drift []Drift
	refused, linked := g.outputSafety(outputs)
	for _, r := range refused {
		blocked[r.rel] = true
	}
	for _, output := range outputs {
		if linked[g.relSlash(g.absOutputPath(output.Path))] {
			continue
		}
		kind, _, ok := g.outputState(output)
		if !ok || kind == "" {
			continue
		}
		rel := g.relSlash(g.absOutputPath(output.Path))
		if blocked[rel] {
			continue
		}
		drift = append(drift, Drift{Path: rel, Kind: kind})
	}
	for rel := range blocked {
		drift = append(drift, Drift{Path: rel, Kind: DriftBlocked})
	}
	for _, stale := range g.retiredFiles(outputs) {
		drift = append(drift, Drift{Path: g.relSlash(stale), Kind: DriftOrphan})
	}
	// A merged document that still holds what an earlier run wrote and this one
	// no longer does (a role or setting that was removed) is rewritten by generate.
	for _, edit := range g.planUnmerge(outputs, false) {
		kind := DriftStale
		if edit.delete {
			kind = DriftOrphan
		}
		drift = append(drift, Drift{Path: edit.rel, Kind: kind})
	}
	sortDrift(drift)
	return &ApplyResult{Drift: drift}, nil
}

type describeApplier struct{}

func (describeApplier) lifecycle() runLifecycle {
	return runLifecycle{downgrades: true, forgetState: true}
}

func (describeApplier) apply(g *Generator, p *RunPlan) (*ApplyResult, error) {
	outputs := p.Outputs
	g.markSensitiveOutputs(outputs)
	g.markPlannedSecretDocuments(outputs)

	doc := &Plan{Schema: PlanSchema, Renderer: templates.GeneratorSchemaVersion, Files: []PlanFile{}, Removals: []PlanRemoval{}}
	if role := g.Role(); role != "" {
		doc.Role = role
	} else {
		doc.Profile = p.Profile
	}
	for i := range outputs {
		doc.Files = append(doc.Files, g.planFile(&outputs[i]))
	}
	sort.Slice(doc.Files, func(i, j int) bool { return doc.Files[i].Path < doc.Files[j].Path })

	for _, stale := range g.retiredFiles(outputs) {
		doc.Removals = append(doc.Removals, PlanRemoval{Path: g.relSlash(stale), Reason: RemoveStale})
	}
	for _, edit := range g.planUnmerge(outputs, false) {
		reason := RemoveUnmerge
		if edit.delete {
			reason = RemoveDelete
		}
		doc.Removals = append(doc.Removals, PlanRemoval{Path: edit.rel, Reason: reason})
	}
	sort.Slice(doc.Removals, func(i, j int) bool {
		if doc.Removals[i].Path != doc.Removals[j].Path {
			return doc.Removals[i].Path < doc.Removals[j].Path
		}
		return doc.Removals[i].Reason < doc.Removals[j].Reason
	})
	return &ApplyResult{Document: doc}, nil
}

// diagnostics is the warning collector of the config this Generator renders,
// created on first use. It issues through the host logger when the Generator has
// one, else through the default sink (the CLI's logger).
func (g *Generator) diagnostics() *diag.Collector {
	if g.config.Diag == nil {
		var sink diag.Sink
		if h := g.host(); h.Log != nil {
			sink = func(msg string, args ...any) { h.Log.Warn(msg, args...) }
		}
		g.config.Diag = diag.New(sink)
	}
	return g.config.Diag
}

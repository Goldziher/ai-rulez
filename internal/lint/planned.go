package lint

import (
	"path"
	"slices"
	"strings"
)

// PlannedFiles is what a generate run would write, as the traps see it. The
// generator imports this package, so a trap cannot call the generator; the
// generator implements this interface instead and the command line hands it in
// (WithPlanned). With it, a trap judges a generated file as the harness will read
// it whether or not `generate` has run, so `validate` no longer has to
// follow it.
type PlannedFiles interface {
	// Planned returns the final bytes a run writes at rel, a slash path relative to
	// the project root. ok is false for a path the plan does not write.
	Planned(rel string) (content []byte, ok bool)
	// PlannedPaths lists every file the plan writes, as slash paths relative to the
	// project root.
	PlannedPaths() []string
}

// WithPlanned makes the traps read the files a run would write instead of the
// ones on disk. A file the plan writes and the disk already holds as the work of
// a person (no generated banner) keeps its on-disk content: the run would leave
// it alone.
func WithPlanned(p PlannedFiles) Option { return func(r *runner) { r.planned = p } }

// plannedRels returns the planned files that fall under the lint root, as slash
// paths relative to the tree top, sorted.
func (r *runner) plannedRels() []string {
	if r.planned == nil {
		return nil
	}
	var out []string
	for _, rel := range r.planned.PlannedPaths() {
		rel = path.Clean(rel)
		if rel == "." || strings.HasPrefix(rel, "../") {
			continue
		}
		out = append(out, path.Join(r.baseRel, rel))
	}
	slices.Sort(out)
	return out
}

// plannedContent returns the planned bytes for the file at f (a path relative to
// the tree top) when the traps should judge them instead of the disk's: the plan
// writes the file and the disk has none, or has one ai-rulez generated.
func (r *runner) plannedContent(f string, onDisk bool, onDiskGenerated bool) ([]byte, bool) {
	if r.planned == nil {
		return nil, false
	}
	rel, ok := r.underRoot(f)
	if !ok {
		return nil, false
	}
	content, planned := r.planned.Planned(rel)
	if !planned || (onDisk && !onDiskGenerated) {
		return nil, false
	}
	if len(content) > maxTrapFileBytes {
		content = content[:maxTrapFileBytes]
	}
	return content, true
}

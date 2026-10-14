package lint

import (
	"path"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
)

// SourceOutput is the Source of a finding in a generated file.
const SourceOutput = "output"

// OutputFile is a generated file to lint: the bytes `generate` writes (or would
// write) at Abs, which need not exist on disk yet.
type OutputFile struct {
	Abs  string
	Data []byte
}

// WithOutputs lints generated files besides the authored content: the analyzers
// that judge what a model reads (security, references, descriptions) run over
// each one, and its findings carry Source "output". A file the project authors
// itself is never an output, an output that repeats an authored item is skipped,
// and a finding on an output that an authored file already has is dropped, so a
// problem is counted once however many generated files carry a copy of it.
func WithOutputs(files []OutputFile) Option { return func(r *runner) { r.outputs = files } }

// outputAnalyzer reports whether an analyzer runs over generated files. The
// others (budgets, metadata, duplicates, ...) judge authoring choices a
// generated file does not make.
func outputAnalyzer(name string) bool {
	switch name {
	case AnalyzerSecurity, AnalyzerReferences, AnalyzerDescriptions:
		return true
	}
	return false
}

// checkOutputs runs the item analyzers over the outputs the authored items do
// not already cover.
func (r *runner) checkOutputs() {
	files := r.outputs
	if r.targets != nil {
		files = append(files[:len(files):len(files)], r.targets.Generated...)
	}
	if len(files) == 0 {
		return
	}
	authored := make(map[string]bool, len(r.items))
	bodies := make(map[string]bool, len(r.items))
	for i := range r.items {
		it := &r.items[i]
		authored[gitutil.Resolve(it.abs)] = true
		if it.owned && !it.isDoc {
			bodies[bodyKey(it.cf.Content)] = true
		}
	}
	for _, o := range files {
		kind, ok := classifyOutput(r.rel(o.Abs))
		if !ok || authored[gitutil.Resolve(o.Abs)] {
			continue
		}
		text := string(o.Data)
		cf := config.ContentFileFromText(o.Abs, text)
		if bodies[bodyKey(cf.Content)] {
			continue // a copy of an authored item: its findings sit at the source
		}
		if r.fileCache == nil {
			r.fileCache = map[string]cachedFile{}
		}
		r.fileCache[o.Abs] = cachedFile{raw: text, ok: true}
		it := item{kind: kind, abs: o.Abs, cf: cf, owned: true}
		before := len(r.findings)
		r.checkItem(&it)
		r.tagOutputFindings(before)
	}
	r.dropRepeatedOutputFindings()
}

// classifyOutput names the content kind of a generated file from its path, or
// reports ok false for a file that is not markdown the analyzers read.
func classifyOutput(rel string) (kind string, ok bool) {
	switch strings.ToLower(path.Ext(rel)) {
	case extMD, extMDC:
	default:
		return "", false
	}
	kind, _ = classifyPath(rel)
	return kind, kind != ""
}

// bodyKey normalizes the body of an item for comparing copies of it.
func bodyKey(body string) string { return strings.TrimSpace(strings.ReplaceAll(body, "\r\n", "\n")) }

// tagOutputFindings marks the findings recorded since before as findings of an
// output, keeps the analyzers that apply to outputs and drops their fixes: a
// generated file is never edited, its source is.
func (r *runner) tagOutputFindings(before int) {
	kept := r.findings[:before]
	for _, f := range r.findings[before:] {
		if !outputAnalyzer(AnalyzerFor(f.Code).Name) {
			continue
		}
		f.meta().Source = SourceOutput
		f.meta().Fix = nil
		kept = append(kept, f)
	}
	r.findings = kept
}

// dropRepeatedOutputFindings removes the findings of outputs that repeat a
// finding of an authored file (same rule, same message): an aggregate such as
// AGENTS.md carries the text of many sources, and each problem is reported at
// the source that owns it.
func (r *runner) dropRepeatedOutputFindings() {
	authored := map[string]bool{}
	for i := range r.findings {
		if f := &r.findings[i]; f.Source() != SourceOutput {
			authored[f.Code+"\x00"+f.Message] = true
		}
	}
	kept := r.findings[:0]
	for i := range r.findings {
		f := r.findings[i]
		if f.Source() == SourceOutput && authored[f.Code+"\x00"+f.Message] {
			continue
		}
		kept = append(kept, f)
	}
	r.findings = kept
}

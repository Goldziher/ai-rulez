package generator

import (
	"sort"
	"sync"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
)

// FinalFiles returns the bytes a run writes for each output of the plan, by slash
// path relative to the project root: headers and hashes included, raw payloads
// verbatim. Directories are left out, and so is an output that may carry a secret,
// whose content a caller reading a plan must not see. The bytes are what a run
// writes, not what the disk holds: a file the overwrite guard would skip is still
// listed.
func (p *RunPlan) FinalFiles() map[string][]byte {
	g := p.owner
	g.markSensitiveOutputs(p.Outputs)
	g.markPlannedSecretDocuments(p.Outputs)
	files := make(map[string][]byte, len(p.Outputs))
	for _, o := range p.Outputs {
		if o.IsDir || o.Sensitive {
			continue
		}
		rel := g.relSlash(g.absOutputPath(o.Path))
		if o.RawContent != nil {
			files[rel] = o.RawContent
			continue
		}
		files[rel] = []byte(g.finalContent(o))
	}
	return files
}

// PlannedFiles serves the final bytes of a run's outputs, rendering them on first
// use; it implements lint.PlannedFiles. A render that fails serves nothing, so the
// checks that use it fall back to the files on disk. The render is quiet: it
// issues no advice of its own, which `generate` shows when it really runs.
type PlannedFiles struct {
	cfg   *config.Config
	once  sync.Once
	files map[string][]byte
}

// NewPlannedFiles returns the planned files of cfg's default profile. cfg is not
// modified: the render works on a copy.
func NewPlannedFiles(cfg *config.Config) *PlannedFiles { return &PlannedFiles{cfg: cfg} }

func (p *PlannedFiles) load() {
	p.once.Do(func() {
		cp := *p.cfg
		cp.Hooks = append(cp.Hooks[:0:0], cp.Hooks...)
		cp.Diag = nil
		cp.Host.Log = logger.Discard()
		g := NewGenerator(&cp)
		g.lockRender = true // leave ${VAR} as written: the plan never holds a resolved secret
		g.SetHost(cp.Host)
		g.diagnostics().SetSink(func(string, ...any) {})
		plan, err := g.Plan("")
		if err != nil {
			return
		}
		p.files = plan.FinalFiles()
	})
}

// Planned implements lint.PlannedFiles.
func (p *PlannedFiles) Planned(rel string) ([]byte, bool) {
	p.load()
	content, ok := p.files[rel]
	return content, ok
}

// PlannedPaths implements lint.PlannedFiles.
func (p *PlannedFiles) PlannedPaths() []string {
	p.load()
	paths := make([]string, 0, len(p.files))
	for rel := range p.files {
		paths = append(paths, rel)
	}
	sort.Strings(paths)
	return paths
}

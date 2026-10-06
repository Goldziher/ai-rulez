package rulefiles

import (
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/diag"
)

// The warnings of a generate run (the aggregated downgrade summary, advice shown
// once however many presets raise it) are collected by the run's diag.Collector,
// which hangs off the config (Config.Diag). Nothing here is process-global, so two
// runs in one process never see each other's entries.

// WarnOnlyNegated warns, once per item and run, that a rule or context file
// with only negated globs cannot be scoped by the rule files and stays in the
// root file.
func WarnOnlyNegated(d *diag.Collector, kind string, cf config.ContentFile, rootFile string) {
	d.OnceWarn("negated", kind+" "+cf.Name,
		kind+" \""+cf.Name+"\" has only negated globs, which rule files cannot express; kept in "+rootFile,
		"path", cf.Path)
}

// ReportNotes forwards the notes Render returned for the file at path:
// downgrades are aggregated into the per-run warning, anything else is warned
// about once, naming the file.
func ReportNotes(d *diag.Collector, path string, notes []Note) {
	for _, n := range notes {
		if n.Downgrade {
			d.RecordDowngrade(kindLabel(n.Kind), n.Name, string(n.Mode))
			continue
		}
		d.Raise(n.Text, "file", path)
	}
}

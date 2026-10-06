package rulefiles

import (
	"github.com/Goldziher/ai-rulez/v5/internal/diag"
)

// collector returns a collector whose warnings land in the returned slice.
func collector() (*diag.Collector, *[]string) {
	var warned []string
	return diag.New(func(msg string, _ ...any) { warned = append(warned, msg) }), &warned
}

// registryWith returns an empty Registry that reports through d.
func registryWith(d *diag.Collector) *Registry {
	r := NewRegistry()
	r.diag = d
	return r
}

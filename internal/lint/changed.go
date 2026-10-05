package lint

import (
	"path/filepath"
	"sort"
)

// Changed-only reporting. The whole tree is still linted, so a reference to a
// file is resolved against everything that exists; only the findings are
// narrowed, to files that changed and files that refer to a changed file
// (one hop: a link, a skill or agent name, a skill resource or a hook script).

// dep records that file from refers to file to (both absolute).
func (r *runner) dep(from, to string) {
	if r.deps == nil {
		r.deps = map[string]map[string]struct{}{}
	}
	f, t := r.tree.Rel(from), r.tree.Rel(to)
	if f == "" || t == "" || f == t {
		return
	}
	if r.deps[f] == nil {
		r.deps[f] = map[string]struct{}{}
	}
	r.deps[f][t] = struct{}{}
}

// depName records that from refers to every item answering to name.
func (r *runner) depName(from, name string) {
	for _, p := range r.names[name] {
		r.dep(from, p)
	}
}

func (r *runner) exportDeps() map[string][]string {
	out := make(map[string][]string, len(r.deps))
	for from, tos := range r.deps {
		list := make([]string, 0, len(tos))
		for t := range tos {
			list = append(list, t)
		}
		sort.Strings(list)
		out[from] = list
	}
	return out
}

// ChangedScope is the outcome of narrowing a report to changed files.
type ChangedScope struct {
	// Since is the revision the change set was computed against.
	Since string `json:"since"`
	// Changed and Dependents count the files findings were kept for.
	Changed    int `json:"changed_files"`
	Dependents int `json:"dependent_files"`
	// Dropped counts findings in untouched files that were left out.
	Dropped int `json:"dropped_findings"`
}

// NarrowToChanged keeps only the findings of r located in a changed file or in
// a file that refers to one. changed holds repository-relative slash paths.
func NarrowToChanged(r *Report, changed []string, since string) ChangedScope {
	set := make(map[string]bool, len(changed))
	for _, c := range changed {
		set[filepath.ToSlash(c)] = true
	}
	deps := map[string]bool{}
	for from, tos := range r.Deps {
		if set[from] {
			continue
		}
		for _, t := range tos {
			if set[t] {
				deps[from] = true
				break
			}
		}
	}
	scope := ChangedScope{Since: since, Changed: len(set), Dependents: len(deps)}
	kept := r.Findings[:0:0]
	for i := range r.Findings {
		p := r.Findings[i].RepoPath()
		if set[p] || deps[p] {
			kept = append(kept, r.Findings[i])
		} else {
			scope.Dropped++
		}
	}
	r.Findings = kept
	return scope
}

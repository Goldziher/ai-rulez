package lint

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
)

// Changed-only reporting. The whole tree is still linted, so a reference to a
// file is resolved against everything that exists; only the findings are
// narrowed, to files that changed and files that refer to a changed file
// (a link, a skill or agent name, a skill resource or a hook script; one hop by
// default, transitively with a depth).

// dep records that file from refers to file to (both absolute).
func (r *runner) dep(from, to string) {
	if r.deps == nil {
		r.deps = map[string]map[string]struct{}{}
	}
	f, t := r.rel(from), r.rel(to)
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

// DepthAll expands the changed set to its whole reverse-dependency closure.
const DepthAll = -1

// Hop labels of a finding in changed-only reports.
const (
	HopChanged   = "changed"
	HopDependent = "dependent"
)

// ChangedScope is the outcome of narrowing a report to changed files.
type ChangedScope struct {
	// Since is the revision the change set was computed against.
	Since string `json:"since"`
	// Depth is the number of reference hops followed: 1 is the files that refer
	// to a changed file, DepthAll (-1) the whole closure.
	Depth int `json:"depth"`
	// Changed counts the changed files; Dependents the files one hop away and
	// Transitive those two or more hops away, which findings were kept for.
	Changed    int `json:"changed_files"`
	Dependents int `json:"dependent_files"`
	Transitive int `json:"transitive_files,omitempty"`
	// Truncated counts the farthest files left out because of the file cap.
	Truncated int `json:"truncated_files,omitempty"`
	// Dropped counts findings in untouched files that were left out.
	Dropped int `json:"dropped_findings"`
}

// DepthLabel is the depth as written on the command line: a number or "all".
func (s ChangedScope) DepthLabel() string {
	if s.Depth == DepthAll {
		return "all"
	}
	return strconv.Itoa(s.Depth)
}

// NarrowOptions tunes NarrowToChangedWith.
type NarrowOptions struct {
	// Depth is how many reference hops to follow from the changed files: 1 (the
	// default for 0) the files that refer to them, N further, DepthAll the closure.
	Depth int
	// MaxFiles caps the files reported besides the changed ones (0: no cap). The
	// nearest files are kept, ties broken by path.
	MaxFiles int
}

// NarrowToChanged keeps only the findings of r located in a changed file or in
// a file that refers to one (one hop). changed holds repository-relative slash paths.
func NarrowToChanged(r *Report, changed []string, since string) ChangedScope {
	return NarrowToChangedWith(r, changed, since, NarrowOptions{Depth: 1})
}

// NarrowToChangedWith is NarrowToChanged following references transitively. The
// closure is a breadth-first walk over the reverse reference graph, so a cycle
// ends when no new file is found and the result does not depend on map order.
// Each kept finding is marked with its hop (Finding.Hop).
func NarrowToChangedWith(r *Report, changed []string, since string, opts NarrowOptions) ChangedScope {
	depth := opts.Depth
	if depth == 0 {
		depth = 1
	}
	hops := map[string]int{}
	var frontier []string
	for _, c := range changed {
		c = filepath.ToSlash(c)
		if _, seen := hops[c]; !seen {
			hops[c] = 0
			frontier = append(frontier, c)
		}
	}
	scope := ChangedScope{Since: since, Depth: depth, Changed: len(hops)}
	rev := map[string][]string{}
	for from, tos := range r.Deps {
		for _, t := range tos {
			rev[t] = append(rev[t], from)
		}
	}
	reported := 0
	for hop := 1; len(frontier) > 0 && (depth == DepthAll || hop <= depth); hop++ {
		var next []string
		for _, t := range frontier {
			for _, from := range rev[t] {
				if _, seen := hops[from]; !seen {
					hops[from] = hop
					next = append(next, from)
				}
			}
		}
		sort.Strings(next)
		frontier = keepHop(next, hop, opts.MaxFiles, &reported, hops, &scope)
	}
	kept := r.Findings[:0:0]
	for i := range r.Findings {
		hop, ok := hops[r.Findings[i].RepoPath()]
		if !ok {
			scope.Dropped++
			continue
		}
		r.Findings[i].meta().Hop = hopLabel(hop)
		kept = append(kept, r.Findings[i])
	}
	r.Findings = kept
	return scope
}

// keepHop admits the files first reached at hop, nearest first, until the file
// cap is hit; the files past it are forgotten and counted as truncated. It
// returns the files kept, which are the next frontier.
func keepHop(next []string, hop, maxFiles int, reported *int, hops map[string]int, scope *ChangedScope) []string {
	kept := next[:0]
	for _, f := range next {
		if maxFiles > 0 && *reported >= maxFiles {
			delete(hops, f)
			scope.Truncated++
			continue
		}
		*reported++
		kept = append(kept, f)
		if hop == 1 {
			scope.Dependents++
		} else {
			scope.Transitive++
		}
	}
	return kept
}

func hopLabel(hop int) string {
	switch hop {
	case 0:
		return HopChanged
	case 1:
		return HopDependent
	}
	return fmt.Sprintf("transitive(%d)", hop)
}

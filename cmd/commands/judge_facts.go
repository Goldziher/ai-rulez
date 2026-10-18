package commands

import (
	"context"
	"errors"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/judge"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
)

// loadJudgeProjectConfig loads the project's configuration for the lint run and the [judge] table:
// the real one when the project has it, else none. A project with no configuration is fine; a broken
// one is an error.
func loadJudgeProjectConfigAt(ctx context.Context, root string) (*config.Config, error) {
	cfg, err := loadProject(ctx, root)
	if err != nil {
		if errors.Is(err, config.ErrNoConfig) {
			return nil, nil
		}
		return nil, err
	}
	return cfg, nil
}

// staticFacts runs the validate and scan analyzers over the files of the units and returns their
// findings as leads for the reviewers, by unit id. A fact is never a finding on its own. When the
// analyzers cannot run the units are judged without facts and the caller is told why.
func (w *judgeWorkspace) staticFacts(ctx context.Context, units []judge.Unit, cfg *config.Config) (facts map[string][]judge.Fact, note string) {
	out := map[string][]judge.Fact{}
	owner, abs := w.factOwners(units)
	if len(abs) == 0 {
		return out, ""
	}
	if cfg == nil {
		implicit, err := loadProjectImplicit(ctx, w.root)
		if err != nil {
			return out, factsUnavailable(err.Error())
		}
		cfg = implicit
	}
	report, reason := w.lintFiles(ctx, cfg, abs)
	if reason != "" {
		return out, factsUnavailable(reason)
	}
	byUnit := w.findingsByUnit(report, owner)
	unitByID := map[string]judge.Unit{}
	for i := range units {
		unitByID[units[i].ID] = units[i]
	}
	for id, findings := range byUnit {
		out[id] = factsOf(unitByID[id], findings)
	}
	return out, ""
}

func factsUnavailable(reason string) string { return "static facts unavailable: " + reason }

// factOwners maps each file of a reviewable unit to its unit id and lists the units' primary files.
func (w *judgeWorkspace) factOwners(units []judge.Unit) (owner map[string]string, abs []string) {
	owner = map[string]string{}
	for i := range units {
		u := &units[i]
		if u.Status != "" {
			continue
		}
		for _, f := range u.Files {
			owner[f.Path] = u.ID
		}
		abs = append(abs, filepath.Join(w.root, filepath.FromSlash(u.Primary)))
	}
	return owner, abs
}

// lintFiles runs the analyzers over the files; a non-empty reason says why they could not run.
func (w *judgeWorkspace) lintFiles(ctx context.Context, cfg *config.Config, abs []string) (report *lint.Report, reason string) {
	targets, err := lint.ResolveTargets(ctx, w.tree, lint.TargetSpec{Paths: abs, Cwd: w.root, Lenient: true})
	if err != nil {
		return nil, err.Error()
	}
	report, err = lint.RunWith(cfg, w.tree, lint.Options{Cwd: w.root}, lint.WithTargets(targets), lint.WithContext(ctx))
	if err != nil {
		return nil, err.Error()
	}
	if report == nil {
		return nil, "no report"
	}
	return report, ""
}

// findingsByUnit groups the findings by the unit that owns their file, with the file made project-relative.
func (w *judgeWorkspace) findingsByUnit(report *lint.Report, owner map[string]string) map[string][]lint.Finding {
	byUnit := map[string][]lint.Finding{}
	for _, f := range report.Findings {
		rel := f.File
		if f.Meta != nil && f.Meta.Path != "" {
			rel = f.Meta.Path
		} else if filepath.IsAbs(rel) {
			if r, ok := w.rel(rel); ok {
				rel = r
			}
		}
		rel = filepath.ToSlash(rel)
		if id, ok := owner[rel]; ok {
			f.File = rel
			byUnit[id] = append(byUnit[id], f)
		}
	}
	return byUnit
}

// factsOf turns lint findings into facts: worst first, then by place, numbered F1..Fn.
func factsOf(u judge.Unit, findings []lint.Finding) []judge.Fact {
	rank := func(s lint.Severity) int {
		switch s {
		case lint.SeverityError:
			return 3
		case lint.SeverityWarning:
			return 2
		case lint.SeverityInfo:
			return 1
		}
		return 0
	}
	sort.SliceStable(findings, func(i, j int) bool {
		a, b := findings[i], findings[j]
		switch {
		case rank(a.Severity) != rank(b.Severity):
			return rank(a.Severity) > rank(b.Severity)
		case a.File != b.File:
			return a.File < b.File
		case a.Line != b.Line:
			return a.Line < b.Line
		}
		return a.Code < b.Code
	})
	facts := make([]judge.Fact, 0, len(findings))
	for i, f := range findings {
		fact := judge.Fact{ID: "F" + strconv.Itoa(i+1), Code: f.Code, Severity: string(f.Severity), Path: f.File, Line: f.Line, Message: judge.SafeText(truncateMessage(f.Message, 200))}
		for _, uf := range u.Files {
			if uf.Path == f.File {
				if f.Line >= 1 && f.Line <= len(uf.Lines) {
					fact.Quote = judge.SafeText(truncateMessage(strings.TrimSpace(uf.Lines[f.Line-1]), 200))
				}
				fact.ContextKind = contextKind(uf.Role, f.File)
			}
		}
		facts = append(facts, fact)
	}
	return facts
}

func contextKind(role, p string) string {
	switch role {
	case "script":
		return "code"
	case typeConfig:
		return typeConfig
	}
	if strings.EqualFold(path.Ext(p), ".json") {
		return typeConfig
	}
	return "prose"
}

func truncateMessage(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}

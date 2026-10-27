package judgebench

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// The competitor adapter runs an open-source scanner in its offline mode over the same corpus and
// turns what it prints into a judge Report, so the same labels score it. The scanner is not told about
// the labels, does not call a model (no cost), and is read as a security scanner is read: a finding of
// medium severity or above counts, after its category is mapped onto the nearest JR-S code.
//
//   - Cisco AI Defense skill-scanner (Apache-2.0, Copyright 2026 Cisco Systems, Inc. and its affiliates)
//
// It scans skill directories only, so units of another kind (agents, commands, rules, CLAUDE.md,
// MCP and settings files) have no report unit and count as missed; SkillUnits narrows a comparison
// to the units it can see.

// The security codes the other scanners' categories map onto.
const (
	codeInjection     = "JR-S01"
	codeObfuscation   = "JR-S02"
	codeExfiltration  = "JR-S03"
	codeDangerousCode = "JR-S05"
	codePrivilege     = "JR-S06"
	codePersistence   = "JR-S09"

	// statusPass is the status of a report unit that was reviewed and met the gate.
	statusPass = "pass"
)

// ciscoCodes maps a Cisco skill-scanner category to the nearest security code. A category that is not
// listed (policy_violation, an informational finding) is not a security finding.
var ciscoCodes = map[string]string{
	"prompt_injection":      codeInjection,
	"social_engineering":    codeInjection,
	"obfuscation":           codeObfuscation,
	"hidden_content":        codeObfuscation,
	"data_exfiltration":     codeExfiltration,
	"credential_theft":      "JR-S04",
	"command_injection":     codeDangerousCode,
	"malware":               codeDangerousCode,
	"unauthorized_tool_use": codePrivilege,
	"tool_poisoning":        "JR-S07",
	"persistence":           codePersistence,
	"supply_chain_attack":   "JR-S10",
	"hardcoded_secrets":     "JR-S14",
	"resource_abuse":        "JR-S15",
}

// atLeastMedium reports whether a severity name is medium or above.
func atLeastMedium(sev string) bool { return rank(strings.ToLower(sev)) >= rank("medium") }

// SkillDirs lists the skill directories of the corpus (slash paths relative to its root).
func SkillDirs(corpus string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(corpus, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && d.Name() == "SKILL.md" {
			rel, rerr := filepath.Rel(corpus, filepath.Dir(p))
			if rerr != nil {
				return rerr
			}
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	sort.Strings(out)
	if err != nil {
		return nil, fmt.Errorf("walk the corpus: %w", err)
	}
	return out, nil
}

// SkillUnits returns the ids of the manifest units that hold a skill, the only ones the competitors can see.
func SkillUnits(m Manifest) (map[string]bool, error) {
	dirs, err := SkillDirs(m.CorpusDir())
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for i := range m.Units {
		u := &m.Units[i]
		for _, d := range dirs {
			if d == u.Path() || strings.HasPrefix(d, u.Path()+"/") {
				out[u.ID] = true
				break
			}
		}
	}
	return out, nil
}

func newFinding(code, sev, file, quote string, line int) Finding {
	var f Finding
	f.Code, f.Category, f.Severity, f.Quote = code, category(code), strings.ToLower(sev), quote
	f.Location.Path, f.Location.StartLine = file, line
	return f
}

// RunCisco runs `skill-scanner scan-all` (static analyzers, no model) over the skills of the corpus.
func RunCisco(ctx context.Context, bin, corpus, outDir string) (*Report, error) {
	corpus, err := filepath.Abs(corpus)
	if err != nil {
		return nil, fmt.Errorf("resolve the corpus: %w", err)
	}
	dirs, err := SkillDirs(corpus)
	if err != nil {
		return nil, err
	}
	links, err := os.MkdirTemp("", "judgebench-cisco-")
	if err != nil {
		return nil, fmt.Errorf("make a directory: %w", err)
	}
	defer os.RemoveAll(links) //nolint:errcheck // a temporary directory
	byLink := map[string]string{}
	for i, d := range dirs {
		name := fmt.Sprintf("s%03d", i)
		if err := os.Symlink(filepath.Join(corpus, filepath.FromSlash(d)), filepath.Join(links, name)); err != nil {
			return nil, fmt.Errorf("link %s: %w", d, err)
		}
		byLink[name] = d
	}
	out := filepath.Join(outDir, "cisco.json")
	cmd := exec.CommandContext(ctx, bin, "scan-all", links, "--policy", "balanced", "--format", "json", "--output", out, "--lenient") //nolint:gosec // the benchmark runs the scanner it was given
	cmd.Env = append(os.Environ(), "LITELLM_LOCAL_MODEL_COST_MAP=True")
	// A scanner may exit non-zero because it found something; only a missing report is a failure.
	b, runErr := cmd.CombinedOutput()
	data, err := os.ReadFile(out) //nolint:gosec // the file the scanner just wrote
	if err != nil {
		return nil, fmt.Errorf("skill-scanner wrote no report: %w (%s)\n%s", err, fmt.Sprint(runErr), b)
	}
	var doc struct {
		Results []struct {
			SkillPath string `json:"skill_path"`
			Findings  []struct {
				RuleID   string `json:"rule_id"`
				Category string `json:"category"`
				Severity string `json:"severity"`
				File     string `json:"file_path"`
				Line     *int   `json:"line_number"`
				Snippet  string `json:"snippet"`
				Title    string `json:"title"`
			} `json:"findings"`
		} `json:"results"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse the scanner report: %w", err)
	}
	rep := &Report{}
	for _, r := range doc.Results {
		dir := byLink[filepath.Base(r.SkillPath)]
		ru := ReportUnit{Path: dir, Status: statusPass, Complete: true}
		for _, f := range r.Findings {
			code, ok := ciscoCodes[strings.ToLower(f.Category)]
			if !ok || !atLeastMedium(f.Severity) {
				continue
			}
			line := 0
			if f.Line != nil {
				line = *f.Line
			}
			quote := f.Snippet
			if quote == "" {
				quote = f.Title
			}
			ru.Findings = append(ru.Findings, newFinding(code, f.Severity, path(dir, f.File), quote, line))
		}
		rep.Units = append(rep.Units, ru)
	}
	return rep, nil
}

func path(dir, file string) string { return filepath.ToSlash(filepath.Join(dir, file)) }

// RunSkillSpector no longer runs anything: SkillSpector support was removed in 5.8.0. It stays so
// the module's exported API does not shrink inside v5.
//
// Deprecated: SkillSpector is not a competitor any more; use RunCisco.
func RunSkillSpector(_ context.Context, _, _, _ string) (*Report, error) {
	return nil, fmt.Errorf("SkillSpector support was removed in ai-rulez 5.8.0")
}

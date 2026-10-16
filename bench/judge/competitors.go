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
	"sync"
)

// The competitor adapters run two open-source scanners in their offline modes over the same corpus
// and turn what they print into a judge Report, so the same labels score them. Neither is told about
// the labels, neither calls a model (no cost), and both are read as a security scanner is read: a
// finding of medium severity or above counts, after its category is mapped onto the nearest JR-S code.
//
//   - Cisco AI Defense skill-scanner (Apache-2.0, Copyright 2026 Cisco Systems, Inc. and its affiliates)
//   - SkillSpector (Apache-2.0, Copyright 2026 NVIDIA CORPORATION & AFFILIATES)
//
// Both scan skill directories only, so units of another kind (agents, commands, rules, CLAUDE.md,
// MCP and settings files) have no report unit and count as missed; SkillUnits narrows a comparison
// to the units they can see.

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

// skillSpectorCodes maps a SkillSpector category to the nearest security code.
var skillSpectorCodes = map[string]string{
	"prompt injection":       codeInjection,
	"anti-refusal":           codeInjection,
	"data exfiltration":      codeExfiltration,
	"credential access":      "JR-S04",
	"dangerous code":         codeDangerousCode,
	"output handling":        codeDangerousCode,
	"tool misuse":            codeDangerousCode,
	"privilege escalation":   codePrivilege,
	"excessive agency":       codePrivilege,
	"mcp least privilege":    codePrivilege,
	"mcp tool poisoning":     "JR-S07",
	"taint tracking":         "JR-S08",
	"memory poisoning":       codePersistence,
	"rogue agent":            codePersistence,
	"supply chain":           "JR-S10",
	"trigger abuse":          "JR-S13",
	"hardcoded secrets":      "JR-S14",
	"yara signatures":        codeDangerousCode,
	"system prompt leakage":  codeExfiltration,
	"resource abuse":         "JR-S15",
	"obfuscation":            codeObfuscation,
	"hidden content":         codeObfuscation,
	"social engineering":     codeInjection,
	"malicious code pattern": codeDangerousCode,
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

// RunSkillSpector runs `skillspector scan --no-llm` on every skill of the corpus, a few at a time. The
// scanner looks vulnerable dependencies up on a public service even without a model; the environment
// points its HTTPS proxy at a closed port so nothing leaves the machine.
func RunSkillSpector(ctx context.Context, bin, corpus, outDir string) (*Report, error) {
	corpus, err := filepath.Abs(corpus)
	if err != nil {
		return nil, fmt.Errorf("resolve the corpus: %w", err)
	}
	dirs, err := SkillDirs(corpus)
	if err != nil {
		return nil, err
	}
	rep := &Report{Units: make([]ReportUnit, len(dirs))}
	var wg sync.WaitGroup
	sem := make(chan struct{}, 6)
	var mu sync.Mutex
	var firstErr error
	for i, d := range dirs {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			ru, rerr := skillSpectorOne(ctx, bin, corpus, outDir, i, d)
			mu.Lock()
			defer mu.Unlock()
			if rerr != nil && firstErr == nil {
				firstErr = rerr
			}
			rep.Units[i] = ru
		}()
	}
	wg.Wait()
	return rep, firstErr
}

func skillSpectorOne(ctx context.Context, bin, corpus, outDir string, i int, dir string) (ReportUnit, error) {
	ru := ReportUnit{Path: dir, Status: statusPass, Complete: true}
	out := filepath.Join(outDir, fmt.Sprintf("skillspector-%03d.json", i))
	cmd := exec.CommandContext(ctx, bin, "scan", filepath.Join(corpus, filepath.FromSlash(dir)), "--no-llm", "--format", "json", "--output", out) //nolint:gosec // the benchmark runs the scanner it was given
	cmd.Env = append(os.Environ(), "HTTPS_PROXY=http://127.0.0.1:9", "HTTP_PROXY=http://127.0.0.1:9")
	// SkillSpector exits non-zero for a risky skill; only a missing report is a failure.
	b, runErr := cmd.CombinedOutput()
	data, err := os.ReadFile(out) //nolint:gosec // the file the scanner just wrote
	if err != nil {
		return ru, fmt.Errorf("skillspector wrote no report for %s: %w (%s)\n%s", dir, err, fmt.Sprint(runErr), b)
	}
	var doc struct {
		Issues []struct {
			Category string `json:"category"`
			Severity string `json:"severity"`
			Finding  string `json:"finding"`
			Snippet  string `json:"code_snippet"`
			Location struct {
				File string `json:"file"`
				Line int    `json:"start_line"`
			} `json:"location"`
		} `json:"issues"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return ru, fmt.Errorf("parse the scanner report of %s: %w", dir, err)
	}
	for _, is := range doc.Issues {
		code, ok := skillSpectorCodes[strings.ToLower(is.Category)]
		if !ok || !atLeastMedium(is.Severity) {
			continue
		}
		quote := is.Finding
		if quote == "" {
			quote = is.Snippet
		}
		ru.Findings = append(ru.Findings, newFinding(code, is.Severity, path(dir, is.Location.File), quote, is.Location.Line))
	}
	return ru, nil
}

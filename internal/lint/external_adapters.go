package lint

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"

	"github.com/samber/oops"
)

// Adapters turn the JSON a non-SARIF scanner prints into findings. They are
// selected with format = "adapter:<name>", version-pinned by the profile that
// names them (the shape each expects is documented in docs/strict-validation.md),
// and as strict as the SARIF reader: a document of another shape is an error
// (AR9E3), never an empty clean result.

const (
	adapterPrefix  = "adapter:"
	adapterSnyk    = "snyk-json"
	adapterClaude  = "claude-validate-json"
	maxAdapterText = 2000
)

// knownAdapters lists the adapter names format accepts.
var knownAdapters = []string{adapterClaude, adapterSnyk}

// adapterName returns the adapter of a format string ("adapter:snyk-json").
func adapterName(format string) (string, bool) {
	name, ok := strings.CutPrefix(strings.ToLower(strings.TrimSpace(format)), adapterPrefix)
	return name, ok
}

// validFormat reports whether format is "", sarif, json or a known adapter.
func validFormat(format string) bool {
	switch strings.ToLower(format) {
	case "", "sarif", "json":
		return true
	}
	name, ok := adapterName(format)
	if !ok {
		return false
	}
	for _, k := range knownAdapters {
		if k == name {
			return true
		}
	}
	return false
}

// parseAdapter runs the named adapter over a scanner's stdout.
func parseAdapter(name string, out []byte, exitCode int, keepSuppressed bool) ([]externalFinding, error) {
	if len(bytes.TrimSpace(out)) == 0 {
		return nil, oops.Errorf("no output")
	}
	switch name {
	case adapterClaude:
		return parseClaudeValidate(out)
	case adapterSnyk:
		return parseSnyk(out, exitCode)
	}
	return nil, oops.Errorf("unknown adapter %q (use %s)", name, strings.Join(knownAdapters, ", "))
}

// claudeValidateIssue is one error, warning or note of `claude plugin validate --json`.
type claudeValidateIssue struct {
	Path    string  `json:"path"`
	Message string  `json:"message"`
	Code    *string `json:"code"`
}

type claudeValidateEntry struct {
	File     string                `json:"file"`
	Type     string                `json:"type"`
	Errors   []claudeValidateIssue `json:"errors"`
	Warnings []claudeValidateIssue `json:"warnings"`
	Notes    []claudeValidateIssue `json:"notes"`
}

// parseClaudeValidate reads `claude plugin validate --json`:
//
//	{"success": bool, "manifest": {file,type,errors,warnings,notes} | null,
//	 "contents": [{file,type,errors,warnings,notes}]}
//
// Errors map to error, warnings to warning and notes to note. A report with
// success = false and no error is a failed run, not a clean one.
func parseClaudeValidate(out []byte) ([]externalFinding, error) {
	var doc struct {
		Success  *bool                 `json:"success"`
		Manifest *claudeValidateEntry  `json:"manifest"`
		Contents []claudeValidateEntry `json:"contents"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		return nil, oops.Wrapf(err, "invalid claude-validate-json")
	}
	if doc.Success == nil {
		return nil, oops.Errorf("not a claude plugin validate report (no success field)")
	}
	entries := doc.Contents
	if doc.Manifest != nil {
		entries = append([]claudeValidateEntry{*doc.Manifest}, entries...)
	}
	var found []externalFinding
	errCount := 0
	for _, e := range entries {
		for _, group := range []struct {
			issues []claudeValidateIssue
			level  string
		}{{e.Errors, "error"}, {e.Warnings, "warning"}, {e.Notes, "note"}} {
			for _, is := range group.issues {
				if len(found) >= maxScannerResults {
					return nil, oops.Errorf("more than %d results", maxScannerResults)
				}
				if group.level == "error" {
					errCount++
				}
				rule := is.Path
				if is.Code != nil && *is.Code != "" {
					rule = *is.Code
				}
				found = append(found, externalFinding{File: e.File, Severity: group.level,
					Rule: e.Type + ":" + rule, Message: is.Message})
			}
		}
	}
	if !*doc.Success && errCount == 0 {
		return nil, oops.Errorf("the validator reported failure without naming an error")
	}
	return found, nil
}

// snykIssue is one issue of a Snyk agent-scan report; the first spelling of
// each field that is set wins.
type snykIssue struct {
	ID          string `json:"id"`
	Code        string `json:"code"`
	Rule        string `json:"rule"`
	Severity    string `json:"severity"`
	Level       string `json:"level"`
	Title       string `json:"title"`
	Message     string `json:"message"`
	Description string `json:"description"`
	File        string `json:"file"`
	Path        string `json:"path"`
	Filename    string `json:"filename"`
	Line        int    `json:"line"`
	StartLine   int    `json:"start_line"`
}

func (i snykIssue) finding(file string) externalFinding {
	first := func(vs ...string) string {
		for _, v := range vs {
			if v != "" {
				return v
			}
		}
		return ""
	}
	f := externalFinding{
		File:     first(i.File, i.Path, i.Filename, file),
		Line:     max(i.Line, i.StartLine),
		Severity: first(i.Severity, i.Level),
		Rule:     first(i.ID, i.Code, i.Rule),
		Message:  first(i.Message, i.Description, i.Title),
	}
	if len(f.Message) > maxAdapterText {
		f.Message = f.Message[:maxAdapterText]
	}
	return f
}

// parseSnyk reads a Snyk agent-scan JSON report in any of three shapes:
// {"issues": [...]}, a bare list of issues, or {"<file>": {"issues": [...]}, ...}.
// Severities critical, high, medium, low and info are kept (anything else counts
// as medium). A document with none of these shapes, or an empty one from a
// scanner that exited non-zero, is an error.
func parseSnyk(out []byte, exitCode int) ([]externalFinding, error) {
	var list []snykIssue
	var top map[string]json.RawMessage
	var found []externalFinding
	add := func(file string, issues []snykIssue) error {
		for _, is := range issues {
			if len(found) >= maxScannerResults {
				return oops.Errorf("more than %d results", maxScannerResults)
			}
			found = append(found, is.finding(file))
		}
		return nil
	}
	switch {
	case json.Unmarshal(out, &list) == nil:
		if err := add("", list); err != nil {
			return nil, err
		}
	case json.Unmarshal(out, &top) == nil:
		if raw, ok := top["issues"]; ok {
			if err := json.Unmarshal(raw, &list); err != nil {
				return nil, oops.Wrapf(err, "invalid snyk-json issues")
			}
			if err := add("", list); err != nil {
				return nil, err
			}
			break
		}
		files := make([]string, 0, len(top))
		for k := range top {
			files = append(files, k)
		}
		sort.Strings(files)
		recognised := false
		for _, file := range files {
			var entry struct {
				Issues *[]snykIssue `json:"issues"`
			}
			if json.Unmarshal(top[file], &entry) != nil || entry.Issues == nil {
				continue
			}
			recognised = true
			if err := add(file, *entry.Issues); err != nil {
				return nil, err
			}
		}
		if !recognised {
			return nil, oops.Errorf("not a snyk-json report (no issues list)")
		}
	default:
		return nil, oops.Errorf("invalid snyk-json: expected an object or a list")
	}
	if len(found) == 0 && exitCode != 0 {
		return nil, oops.Errorf("the scanner printed no issues and exited with status %d", exitCode)
	}
	return found, nil
}

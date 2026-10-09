package handlers

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// ValidateParams are the lint selectors of validate_config; each is the flag of
// `ai-rulez validate` of the same name.
type ValidateParams struct {
	// FailOn is --fail-on: error, warning, info or none; empty resolves like the CLI.
	FailOn string
	// Strict is --strict: the same as FailOn "warning".
	Strict bool
	// ConfigOnly is --config-only: skip the content checks.
	ConfigOnly bool
	// LintProfile is --lint-profile.
	LintProfile string
	// Analyzers is --analyzer.
	Analyzers []string
	// SecurityOnly runs the security checks alone, as `ai-rulez scan` does.
	SecurityOnly bool
}

// ValidateOutcome is what a Validator found.
type ValidateOutcome struct {
	// Report is the combined lint report; empty for a ConfigOnly run.
	Report lint.Combined
	// FailOn is the threshold that was applied.
	FailOn string
	// Failed reports that the findings reach FailOn (the CLI exits 2).
	Failed bool
}

// Validator lints a loaded configuration. The CLI supplies the engine behind
// `ai-rulez validate`, so validate_config and the command report the same
// findings with the same thresholds; a failure to run it is an error.
type Validator func(ctx context.Context, cfg *config.Config, params ValidateParams) (*ValidateOutcome, error)

// ValidateConfigWith returns the validate_config handler: the structural checks
// of ValidateConfigHandler, then the lint of validate. A configuration that does
// not validate, or whose findings reach fail_on, is an error result; a clean one
// is a success result with the findings below the threshold as warnings.
func ValidateConfigWith(validate Validator) func(context.Context, *ToolRequest) (*sdkmcp.CallToolResult, error) {
	return lintWith(validate, false)
}

// ScanContentWith returns the scan_content handler: the security checks of
// `ai-rulez scan` (secrets, hidden text, injection phrases, risky shell, unpinned
// remote sources) with the verdict and findings document of validate_config.
func ScanContentWith(validate Validator) func(context.Context, *ToolRequest) (*sdkmcp.CallToolResult, error) {
	return lintWith(validate, true)
}

func lintWith(validate Validator, securityOnly bool) func(context.Context, *ToolRequest) (*sdkmcp.CallToolResult, error) {
	return func(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
		cfg, err := validateStructure(ctx, request)
		if err != nil {
			return invalidConfig(err)
		}
		if validate == nil {
			return ToolError(errors.New("this server was built without the lint engine"))
		}
		params := ValidateParams{
			FailOn:      request.GetString("fail_on", ""),
			Strict:      request.GetBool("strict", false),
			ConfigOnly:  request.GetBool("config_only", false),
			LintProfile: request.GetString("lint_profile", ""),
			Analyzers:   request.GetStringSlice("analyzers", nil),
		}
		if securityOnly {
			params = ValidateParams{
				FailOn:       request.GetString("fail_on", ""),
				LintProfile:  request.GetString("lint_profile", ""),
				SecurityOnly: true,
			}
		}
		outcome, err := validate(ctx, cfg, params)
		if err != nil {
			return invalidConfig(err)
		}
		return validateResult(cfg, outcome)
	}
}

// relativeTo shows the paths of a report relative to the project directory, as
// the command shows them relative to the directory it runs in. The lint engine
// reports the absolute paths of the project it was handed; the server's own
// working directory says nothing about it.
func relativeTo(base string, report lint.Combined) lint.Combined {
	rel := func(path string) string {
		if path == "" || !filepath.IsAbs(path) {
			return path
		}
		r, err := filepath.Rel(base, path)
		if err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
			return path
		}
		return filepath.ToSlash(r)
	}
	out := report
	out.Roots = make([]string, len(report.Roots))
	for i, root := range report.Roots {
		out.Roots[i] = rel(root)
	}
	out.Findings = make([]lint.Finding, len(report.Findings))
	for i, f := range report.Findings {
		f.File, f.Root = rel(f.File), rel(f.Root)
		out.Findings[i] = f
	}
	return out
}

// validateResult renders a lint outcome as the validate_config document.
func validateResult(cfg *config.Config, outcome *ValidateOutcome) (*sdkmcp.CallToolResult, error) {
	outcome.Report = relativeTo(cfg.BaseDir, outcome.Report)
	warnings, errs := []string{}, []string{}
	for i := range outcome.Report.Findings {
		f := &outcome.Report.Findings[i]
		line := fmt.Sprintf("%s:%d: %s %s", f.File, f.Line, f.Code, f.Message)
		switch f.Severity {
		case lint.SeverityError:
			errs = append(errs, line)
		case lint.SeverityWarning:
			warnings = append(warnings, line)
		}
	}
	doc := map[string]interface{}{
		keyValid:   !outcome.Failed,
		"fail_on":  outcome.FailOn,
		keyConfig:  cfg.ConfigDir,
		"warnings": warnings,
		"errors":   errs,
	}
	if len(outcome.Report.Roots) > 0 {
		// The whole lint document of `validate --format json`, so a field the
		// engine adds (baseline, ratchet, analyzers, changed_only) reaches the tool.
		lintDoc, err := document(outcome.Report)
		if err != nil {
			return ToolError(err)
		}
		for key, value := range lintDoc {
			if _, taken := doc[key]; !taken {
				doc[key] = value
			}
		}
	}
	return reportResult(doc, outcome.Failed)
}

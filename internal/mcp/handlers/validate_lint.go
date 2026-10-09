package handlers

import (
	"context"
	"errors"
	"fmt"

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
	return func(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
		cfg, err := validateStructure(ctx, request)
		if err != nil {
			return invalidConfig(err)
		}
		if validate == nil {
			return ToolError(errors.New("validate_config cannot lint: this server was built without the lint engine"))
		}
		params := ValidateParams{
			FailOn:      request.GetString("fail_on", ""),
			Strict:      request.GetBool("strict", false),
			ConfigOnly:  request.GetBool("config_only", false),
			LintProfile: request.GetString("lint_profile", ""),
			Analyzers:   request.GetStringSlice("analyzers", nil),
		}
		outcome, err := validate(ctx, cfg, params)
		if err != nil {
			return invalidConfig(err)
		}
		return validateResult(cfg, outcome)
	}
}

// validateResult renders a lint outcome as the validate_config document.
func validateResult(cfg *config.Config, outcome *ValidateOutcome) (*sdkmcp.CallToolResult, error) {
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
		doc["roots"] = outcome.Report.Roots
		doc["findings"] = outcome.Report.Findings
		doc["summary"] = outcome.Report.Summary
		if outcome.Report.Risk != nil {
			doc["risk"] = outcome.Report.Risk
		}
	}
	if outcome.Failed {
		return toolErrorDocument(doc)
	}
	return ToolSuccess(doc)
}

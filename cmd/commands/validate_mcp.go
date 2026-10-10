package commands

import (
	"context"
	"path/filepath"
	"sync"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/mcp/handlers"
	"github.com/Goldziher/ai-rulez/v5/schema"
)

// mcpValidator is the lint engine behind the MCP validate_config tool. It runs
// the same code as `ai-rulez validate`: schema check, local overlay and include
// checks, strictLint (every analyzer, the governance findings, the baseline and
// the ratchets) and the fail-on judgement of judgeStrict. The command keeps its
// flag state in package variables, so a run borrows them under a lock and puts
// them back; the server may call tools concurrently.
type mcpValidator struct {
	mu sync.Mutex
}

// validate implements handlers.Validator. cfg has already passed the structural
// checks of the tool (load, Validate, organization policy, overlay schema).
func (v *mcpValidator) validate(ctx context.Context, cfg *config.Config, p handlers.ValidateParams) (*handlers.ValidateOutcome, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	defer v.borrowFlags(p)()

	if err := mcpValidateFlags(p); err != nil {
		return nil, err
	}
	if cfg.ConfigFile != "" {
		if err := schema.ValidateFile(filepath.Join(cfg.ConfigDir, cfg.ConfigFile)); err != nil {
			return nil, schemaFailure(cfg, err)
		}
	}
	if err := checkLocalIncludes(cfg); err != nil {
		return nil, err
	}
	failOn := failOnFor(cfg)
	if p.ConfigOnly {
		return &handlers.ValidateOutcome{FailOn: failOn}, nil
	}

	cfg.DeferMalformedFrontmatter = true // reported as the AR306 finding, as `validate` does
	report, err := strictLint(ctx, cfg)
	if err != nil {
		return nil, err
	}
	reports, cfgs := []*lint.Report{report}, []*config.Config{cfg}
	verdict, _, err := judgeStrict(reports, cfgs) //nolint:contextcheck // the baseline and fix steps run on the command context, not this request's
	if err != nil {
		return nil, err
	}
	return &handlers.ValidateOutcome{Report: verdict.combined, FailOn: failOn, Failed: verdict.code != 0}, nil
}

// mcpValidateFlags rejects the selector values and combinations `validate`
// rejects, with its messages.
func mcpValidateFlags(p handlers.ValidateParams) error {
	if p.Strict && p.ConfigOnly {
		return oops.Errorf("strict (warnings fail) needs the content checks: drop config_only")
	}
	if p.Strict && p.FailOn != "" && p.FailOn != failOnWarning {
		return oops.Errorf("strict means fail_on warning and conflicts with fail_on %s", p.FailOn)
	}
	return checkFlagValues()
}

// borrowFlags points the validate command's flag variables at the call's
// selectors and returns the function that restores them. It also starts every
// call with an empty file-tree cache: the server outlives the files it lints.
func (v *mcpValidator) borrowFlags(p handlers.ValidateParams) (restore func()) {
	savedFailOn, savedProfile, savedAnalyzers := validateFailOn, validateLintProfile, validateAnalyzers
	savedStrict, savedConfigOnly, savedCache := validateStrict, validateConfigOnly, strictTreeCache
	savedSecurity := strictSecurityOnly

	validateFailOn, validateLintProfile, validateAnalyzers = p.FailOn, p.LintProfile, p.Analyzers
	if p.Strict {
		validateFailOn = failOnWarning
	}
	validateConfigOnly, validateStrict = p.ConfigOnly, !p.ConfigOnly
	strictTreeCache = lint.Loader{}
	strictSecurityOnly = p.SecurityOnly
	return func() {
		strictSecurityOnly = savedSecurity
		validateFailOn, validateLintProfile, validateAnalyzers = savedFailOn, savedProfile, savedAnalyzers
		validateStrict, validateConfigOnly, strictTreeCache = savedStrict, savedConfigOnly, savedCache
	}
}

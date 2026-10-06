package commands

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/preflight"
)

const (
	envStrict      = "AI_RULEZ_STRICT"
	envAckCommands = preflight.EnvAck
)

var generateStrict bool

func init() {
	GenerateCmd.Flags().BoolVar(&generateStrict, "strict", false,
		"Fail on unknown or invalid configuration keys instead of warning (env AI_RULEZ_STRICT=1)")
}

func envTrue(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// generatePreflight runs the checks that belong before generate writes anything:
// the schema (unknown keys), the role selection findings, the commands the run is
// about to write, and the hand-written skillOverrides a role replaces. A real run
// (not --dry-run) also records the commands it announced and restores skill
// overrides a role released.
func generatePreflight(cfg *config.Config, gen *generator.Generator) error {
	if err := checkConfigSchema(cfg, generateStrict || envTrue(envStrict)); err != nil {
		return err
	}
	if generateRole != "" {
		warnRoleSelection(cfg, generateRole)
	}
	if pluginMode {
		return nil // bundles carry the [plugin] hooks, not the project's
	}
	warnNewCommands(cfg, os.Stderr, !dryRun)
	if dryRun {
		return nil
	}
	return gen.ReconcileRoleSkillOverrides() //nolint:wrapcheck // already contextual
}

// checkConfigSchema runs the schema validation `validate` runs (shared code in
// config.SchemaFindings, so both commands agree on the unknown keys). Findings
// are warnings, or an error when strict.
func checkConfigSchema(cfg *config.Config, strict bool) error {
	findings, err := config.SchemaFindings(cfg)
	if err != nil {
		return oops.Wrapf(err, "check the configuration against the schema")
	}
	if len(findings) == 0 {
		return nil
	}
	lines := make([]string, len(findings))
	for i, f := range findings {
		lines[i] = f.String()
	}
	if strict {
		return oops.With("errors", lines).
			Hint("Fix the keys above, or run generate without --strict to only warn about them").
			Errorf("configuration has %d unknown or invalid key(s)", len(lines))
	}
	for _, line := range lines {
		logger.Warn("Configuration problem: " + line)
	}
	logger.Warn("These keys have no effect; `ai-rulez validate` fails on them, `generate --strict` (or AI_RULEZ_STRICT=1) does too")
	return nil
}

// warnRoleSelection prints the AR971 findings of the role about to be rendered: a
// domain that does not exist or a selector that matches nothing makes the role
// render less than its author meant, and only validate --strict reported it.
// It uses the same computation as the AR971 lint rule (Config.RoleProblems).
func warnRoleSelection(cfg *config.Config, role string) {
	for _, p := range cfg.RoleProblems() {
		if p.Role == role && p.Kind == config.RoleProblemReference {
			logger.Warn("AR971 " + p.Message)
		}
	}
}

// warnNewCommands prints, to w, the hook, MCP server, env, plugin and allow-rule
// commands this run will write that were not part of the previous run: a
// developer who clones a repository sees what the harness will run. It only
// warns. It is printed even with --quiet because it is security relevant, and
// silenced by --yes or AI_RULEZ_ACK_COMMANDS=1. record stores the commands so
// the next run announces only what changed.
func warnNewCommands(cfg *config.Config, w io.Writer, record bool) {
	lines := preflight.NewCommands(cfg, record)
	if len(lines) > 0 && !assumeYes && !envTrue(envAckCommands) {
		fmt.Fprint(w, preflight.Summary(cfg, lines))
	}
}

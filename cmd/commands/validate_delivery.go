package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator"
	"github.com/Goldziher/ai-rulez/internal/includes"
	"github.com/Goldziher/ai-rulez/internal/lint"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/Goldziher/ai-rulez/internal/mcp"
)

// deliveryFindings computes the dynamic-skill-loading findings of `validate
// --strict` that need the generator or the serving pipeline: the fallback of a
// harness without MCP (AR992), a missing stub (AR991), a missing skills server
// entry (AR993) and lock enforcement (AR995). It returns nothing for a project
// that serves no skill.
func deliveryFindings(cfg *config.Config) []lint.DeliveryFinding {
	served := cfg.DeliveryConfigured(cfg.Content)
	if !served && len(cfg.SkillSources) == 0 {
		return nil
	}
	var out []lint.DeliveryFinding
	for _, fb := range cfg.DeliveryFallbacks(cfg.Content) {
		out = append(out, lint.DeliveryFinding{Code: lint.CodeDeliveryStaticFallback, Message: fmt.Sprintf(
			"preset %q has no MCP support, so its served skills (%s) are written as static files for it instead; no stub is generated there", fb.Preset, strings.Join(fb.Skills, ", "))})
	}
	if served {
		missing, err := generator.NewGenerator(cfg).PresetsMissingStub("")
		if err != nil {
			logger.Warn("Skipped the dynamic-skills stub check", "error", err)
		}
		for _, preset := range missing {
			out = append(out, lint.DeliveryFinding{Code: lint.CodeDeliveryStubMissing, Message: fmt.Sprintf(
				"skills are served but preset %q has no %q stub skill, so its agent is never told to call find_skill; a skill of that name may shadow it, or the preset renders no skills", preset, config.DynamicSkillsName)})
		}
		if hasMCPPreset(cfg) && !runsSkillsServer(cfg) {
			out = append(out, lint.DeliveryFinding{Code: lint.CodeServedNoServer, Message: "skills are served but no [[mcp_servers]] entry runs `ai-rulez mcp --serve-skills`; add one (command \"ai-rulez\", args [\"mcp\", \"--serve-skills\"])"})
		}
	}
	if cfg.LockEnforced() {
		includes.SkipFetch = true
		setup := &mcp.ServeSetup{Version: Version, WorkDir: cfg.BaseDir, NoWatch: true}
		problems, err := setup.ServedProblems(context.Background())
		if err != nil {
			logger.Warn("Skipped the served-skill lock check", "error", err)
		}
		for _, p := range problems {
			out = append(out, lint.DeliveryFinding{Code: lint.CodeServedLockMismatch, Message: p + "; [lock] enforce refuses to serve it (run `ai-rulez lock` after review)"})
		}
	}
	return out
}

func hasMCPPreset(cfg *config.Config) bool {
	for i := range cfg.Presets {
		if config.HarnessSupportsMCP(cfg.Presets[i].GetName()) {
			return true
		}
	}
	return false
}

// runsSkillsServer reports whether some configured MCP server launches the skills server.
func runsSkillsServer(cfg *config.Config) bool {
	launches := func(command string, args []string) bool {
		return strings.Contains(command, "--serve-skills") || containsArg(args, "--serve-skills")
	}
	for i := range cfg.MCPServersRaw {
		if s := &cfg.MCPServersRaw[i]; launches(s.Command, s.Args) {
			return true
		}
	}
	for _, s := range cfg.MCPServers {
		if s != nil && launches(s.Command, s.Args) {
			return true
		}
	}
	return cfg.MCP != nil && launches("", cfg.MCP.SelfServerCommand)
}

func containsArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/includes"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/mcp"
)

// deliveryFindings computes the dynamic-skill-loading findings of `validate
// --strict` that need the generator or the serving pipeline: the fallback of a
// harness without MCP (AR992), a missing stub (AR991), a missing skills server
// entry (AR993) and lock enforcement (AR995). It returns nothing for a project
// that serves no skill.
func deliveryFindings(cfg *config.Config) []lint.DeliveryFinding {
	served := cfg.ServesSkills(cfg.Content) || cfg.RolesServeSkills()
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
		defer func(prev bool) { includes.SkipFetch = prev }(includes.SkipFetch)
		includes.SkipFetch = true
		setup := &mcp.ServeSetup{Version: Version, WorkDir: cfg.BaseDir, NoWatch: true}
		problems, err := setup.ServedProblems(context.Background())
		if err != nil {
			out = append(out, lint.DeliveryFinding{Code: lint.CodeServedLockMismatch, Message: "the served skills could not be checked against " + lockfile.FileName + " offline: " + err.Error() + "; [lock] enforce does not accept an unchecked lock"})
		}
		for _, p := range problems {
			out = append(out, lint.DeliveryFinding{Code: lint.CodeServedLockMismatch, Message: p + "; [lock] enforce refuses to serve it (run `ai-rulez lock` after review)"})
		}
	}
	return append(out, sourceRefusalFindings(cfg)...)
}

// sourceRefusalFindings reports the skills of the skill sources that the security
// scan refuses to serve, which `lock` leaves unpinned. Skills authored in the
// project are reported by the security rules themselves.
func sourceRefusalFindings(cfg *config.Config) []lint.DeliveryFinding {
	if len(cfg.SkillSources) == 0 {
		return nil
	}
	defer func(prev bool) { includes.SkipFetch = prev }(includes.SkipFetch)
	includes.SkipFetch = true
	refusals, err := (&mcp.ServeSetup{Version: Version, WorkDir: cfg.BaseDir, NoWatch: true}).ServedRefusals(context.Background())
	if err != nil {
		logger.Debug("Skipped the skill source refusal check", "error", err.Error())
		return nil
	}
	authored := map[string]bool{}
	if cfg.Content != nil {
		for i := range cfg.Content.Skills {
			authored[config.SkillID(cfg.Content.Skills[i])] = true
		}
		for _, d := range cfg.Content.Domains {
			for i := range d.Skills {
				authored[config.SkillID(d.Skills[i])] = true
			}
		}
	}
	var out []lint.DeliveryFinding
	for _, r := range refusals {
		if !authored[r.Name] {
			out = append(out, lint.DeliveryFinding{Code: r.Code, Message: fmt.Sprintf(
				"served skill %q is refused: %s; `ai-rulez lock` leaves it unpinned", r.Name, r.Reason)})
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

package handlers

import (
	"context"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	incl "github.com/Goldziher/ai-rulez/v5/internal/includes"
	"github.com/Goldziher/ai-rulez/v5/internal/verifiers"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/samber/oops"
)

// mcpDrift is the generated_in_sync check over MCP. Includes and installed
// skills need the network, which the MCP server never uses, so a project that
// declares them cannot be compared here: that is an error, never a pass. Every
// other project gets the real comparison (`generate --check`).
func mcpDrift(cfg *config.Config, profile string) ([]string, error) {
	if len(cfg.Includes) > 0 || len(cfg.InstalledSkills) > 0 {
		return nil, oops.New("includes and installed skills are not resolved over MCP; run `ai-rulez verifiers run`")
	}
	return verifiers.DefaultDrift(cfg, profile)
}

// RunVerifiersHandler evaluates the [[verifiers]] repo checks
// (`ai-rulez verifiers run`) and returns one result per verifier. It never
// writes and never uses the network: includes are not resolved, so a
// generated_in_sync verifier of a project that declares them cannot be
// evaluated here.
func RunVerifiersHandler(ctx context.Context, request *ToolRequest) (*mcp.CallToolResult, error) {
	baseDir := workingDir(request)
	strict := request.GetBool("strict", false)

	cfg, err := loadProjectConfigWith(ctx, request, baseDir, config.WithoutRemote())
	if err != nil {
		return ToolError(err)
	}
	if err := cfg.Validate(); err != nil {
		return ToolError(err)
	}
	var names []string
	if name := request.GetString("name", ""); name != "" {
		names = strings.Split(name, ",")
	}
	report := verifiers.Run(ctx, cfg, verifiers.Options{
		Names: names,
		Drift: mcpDrift,
	})
	if report.Err != nil {
		return ToolError(report.Err)
	}
	for i := range report.Results {
		report.Results[i].Message = incl.RedactURL(report.Results[i].Message)
	}
	counts := report.Counts()
	return ToolSuccess(map[string]interface{}{
		"ok":         !report.CannotRun() && !report.Failed(strict),
		"cannot_run": report.CannotRun(),
		"summary": map[string]int{
			string(verifiers.StatusPass):  counts[verifiers.StatusPass],
			string(verifiers.StatusFail):  counts[verifiers.StatusFail],
			string(verifiers.StatusError): counts[verifiers.StatusError],
		},
		"root":    incl.RedactURL(report.Root),
		"results": report.Results,
	})
}

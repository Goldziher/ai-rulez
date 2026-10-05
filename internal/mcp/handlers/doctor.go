package handlers

import (
	"context"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/doctor"
	incl "github.com/Goldziher/ai-rulez/internal/includes"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// DoctorHandler runs the read-only project diagnostics (`ai-rulez doctor`) and
// returns the findings. It never writes.
func DoctorHandler(ctx context.Context, request *ToolRequest) (*mcp.CallToolResult, error) {
	baseDir := workingDir(request)
	strict := request.GetBool("strict", false)

	report := doctor.Run(ctx, doctor.Options{
		Profile: request.GetString("profile", ""),
		Load: func(ctx context.Context, opts ...config.LoadOption) (*config.Config, error) {
			return loadProjectConfigWith(ctx, request, baseDir, opts...)
		},
	})

	redactReport(report)
	counts := report.Counts()
	return ToolSuccess(map[string]interface{}{
		"ok": !report.Failed(strict),
		"summary": map[string]int{
			string(doctor.SeverityError):   counts[doctor.SeverityError],
			string(doctor.SeverityWarning): counts[doctor.SeverityWarning],
			string(doctor.SeverityInfo):    counts[doctor.SeverityInfo],
		},
		"root":     report.Root,
		"findings": report.Findings,
	})
}

// redactReport strips URL credentials from every free-text field of the report:
// messages, hints and paths can quote a remote include's URL, and the root is a
// path a user might have embedded a credential in.
func redactReport(report *doctor.Report) {
	report.Root = incl.RedactURL(report.Root)
	for i := range report.Findings {
		f := &report.Findings[i]
		f.Message = incl.RedactURL(f.Message)
		f.Hint = incl.RedactURL(f.Hint)
		f.Path = incl.RedactURL(f.Path)
	}
}

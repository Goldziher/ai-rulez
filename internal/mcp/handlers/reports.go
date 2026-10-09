package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/approval"
	"github.com/Goldziher/ai-rulez/v5/internal/builtins"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/cost"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/govview"
	"github.com/Goldziher/ai-rulez/v5/internal/jsondoc"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/okf"
	"github.com/Goldziher/ai-rulez/v5/internal/policy"
	"github.com/Goldziher/ai-rulez/v5/internal/sbom"
	"github.com/Goldziher/ai-rulez/v5/internal/tokens"
	"github.com/Goldziher/ai-rulez/v5/internal/verifiers"
)

// The report tools (token_report, cost_report, sbom, okf_validate,
// approvals_status, policy_show) are read-only twins of the commands of the same
// name. Each returns the document the command prints with --format json, built
// by the same library code; a gate the command turns into exit 2 (an exceeded
// budget, a finding at fail_on) is an error result carrying that document.

// document turns a typed report into the JSON object a tool returns, versioned
// like the command's --format json document (jsondoc: a top-level
// "schema_version"; an array becomes {"schema_version": N, "items": [...]}).
func document(v any) (map[string]any, error) {
	data, err := jsondoc.Marshal(v)
	if err != nil {
		return nil, err //nolint:wrapcheck // already contextual
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, oops.Wrapf(err, "decode json")
	}
	return doc, nil
}

// reportResult answers with doc; failed makes it an error result that still
// carries the document, as the commands print the report before exiting 2.
func reportResult(v any, failed bool) (*sdkmcp.CallToolResult, error) {
	doc, err := document(v)
	if err != nil {
		return ToolError(err)
	}
	if failed {
		return toolErrorDocument(doc)
	}
	return ToolSuccess(doc)
}

type tokenTarget struct{ profile, role string }

// tokenTargets lists what token_report covers, as `tokens` does: the compared
// profiles, one role, or every role.
func tokenTargets(cfg *config.Config, profile, role string, byRole bool, compare []string) ([]tokenTarget, error) {
	if (role != "" || byRole) && (profile != "" || len(compare) > 0) {
		return nil, oops.Hint("Use role or by_role on its own, or profile/compare_profiles").
			Errorf("role and by_role cannot be combined with profile or compare_profiles")
	}
	if role != "" && byRole {
		return nil, oops.Errorf("role and by_role are mutually exclusive")
	}
	switch {
	case byRole:
		names := cfg.RoleNames()
		if len(names) == 0 {
			return nil, oops.Hint("Declare [[roles]] in config.toml").Errorf("no roles are defined")
		}
		targets := make([]tokenTarget, 0, len(names))
		for _, name := range names {
			targets = append(targets, tokenTarget{role: name})
		}
		return targets, nil
	case role != "":
		return []tokenTarget{{role: role}}, nil
	}
	profiles := compare
	if len(profiles) == 0 {
		profiles = []string{profile}
	}
	targets := make([]tokenTarget, 0, len(profiles))
	for _, name := range profiles {
		targets = append(targets, tokenTarget{profile: name})
	}
	return targets, nil
}

// TokenReportHandler is `ai-rulez tokens --format json`: the token surface of
// the generated outputs, rendered in memory. One target answers with the report
// itself, several with {"schema_version": 1, "items": [...]}. A headline over budget is an error
// result carrying the report.
func TokenReportHandler(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	counter, err := tokens.New(request.GetString("tokenizer", ""))
	if err != nil {
		return ToolError(oops.Hint("Accepted values: "+strings.Join(tokens.Names(), ", ")).Wrapf(err, "select tokenizer"))
	}
	cfg, skipped, remote, err := loadOffline(ctx, request, false)
	if err != nil {
		return ToolError(err)
	}
	targets, err := tokenTargets(cfg, request.GetString("profile", ""), request.GetString("role", ""),
		request.GetBool("by_role", false), request.GetStringSlice("compare_profiles", nil))
	if err != nil {
		return ToolError(err)
	}
	reports := make([]*generator.TokenReport, 0, len(targets))
	over := false
	for i, target := range targets {
		if err := ctx.Err(); err != nil {
			return ToolError(err)
		}
		Progress(ctx, float64(i), float64(len(targets)), "token report")
		// A fresh Generator per target: collectOutputs writes SourceHash onto the
		// config it holds, so reusing one would carry a stale hash into the next.
		gen := generator.NewGenerator(cfg)
		gen.SetContext(ctx)
		if target.role != "" {
			if err := gen.SetRole(target.role); err != nil {
				return ToolError(err)
			}
		}
		report, err := gen.TokenReport(generator.TokenReportOptions{
			Profile: target.profile, Counter: counter, Budget: int(request.GetNumber("budget", 0)),
		})
		if err != nil {
			return ToolError(err)
		}
		over = over || (report.Budget != nil && report.Budget.Exceeded)
		reports = append(reports, report)
	}
	var payload any = reports // several reports are the "items" of one document, as in the command
	if len(reports) == 1 {
		payload = reports[0]
	}
	res, err := reportResult(payload, over)
	return withRemoteNote(res, err, skipped || remote)
}

// withRemoteNote appends the note that remote content resolves from the cache only.
func withRemoteNote(res *sdkmcp.CallToolResult, err error, note bool) (*sdkmcp.CallToolResult, error) {
	if err == nil && note && res != nil {
		res.Content = append(res.Content, &sdkmcp.TextContent{Text: remoteSkippedNote})
	}
	return res, err
}

// CostReportHandler is `ai-rulez cost --format json`: which items cost the most
// context. Exceeding budget or on_demand_budget is an error result carrying the report.
func CostReportHandler(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	counter, err := tokens.New(request.GetString("tokenizer", ""))
	if err != nil {
		return ToolError(oops.Hint("Accepted values: "+strings.Join(tokens.Names(), ", ")).Wrapf(err, "select tokenizer"))
	}
	cfg, skipped, remote, err := loadOffline(ctx, request, false)
	if err != nil {
		return ToolError(err)
	}
	report, err := cost.Build(cfg, cost.Options{
		Profile: request.GetString("profile", ""), Target: request.GetString("target", ""),
		Top: int(request.GetNumber("top", costTopDefault)), Counter: counter,
		AlwaysBudget: int(request.GetNumber("budget", 0)), OnDemandBudget: int(request.GetNumber("on_demand_budget", 0)),
	})
	if err != nil {
		return ToolError(err)
	}
	res, err := reportResult(report, report.Exceeded())
	return withRemoteNote(res, err, skipped || remote)
}

// costTopDefault is how many top offenders `cost` lists by default.
const costTopDefault = 10

// SBOMHandler is `ai-rulez sbom`: the CycloneDX or SPDX bill of materials of the
// AI configuration, built from the lock and the cache (nothing is fetched) and
// without the machine-local overlay, as the command does by default.
func SBOMHandler(version string) func(context.Context, *ToolRequest) (*sdkmcp.CallToolResult, error) {
	return func(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
		format, err := sbom.NormalizeFormat(request.GetString("format", ""))
		if err != nil {
			return ToolError(err)
		}
		files := request.GetString("files", "")
		switch files {
		case sbom.FilesNone, sbom.FilesSkills, sbom.FilesAll, "":
		default:
			return ToolError(oops.Hint("use none, skills or all").Errorf("unknown files %q", files))
		}
		ctx = config.WithUnresolvedIncludesTolerated(config.WithOfflineIncludes(ctx))
		cfg, err := loadProjectConfigWith(ctx, request, workingDir(request), config.WithoutLocal())
		if err != nil {
			return ToolError(oops.Hint("sbom reads remote sources from the lock and the cache only; run `ai-rulez generate` or `ai-rulez lock` to fill the cache").Wrap(err))
		}
		if err := cfg.Validate(); err != nil {
			return ToolError(err)
		}
		bom, err := sbom.Build(cfg, version, sbom.Options{
			Files: files, IncludeOutputs: request.GetBool("include_outputs", false),
			Profile: request.GetString("profile", ""), Role: request.GetString("role", ""),
			NoApprovals: request.GetBool("no_approvals", false), Now: govview.ApprovalNow(),
		})
		if err != nil {
			return ToolError(err)
		}
		var buf bytes.Buffer
		if err := sbom.Render(&buf, bom, format); err != nil {
			return ToolError(err)
		}
		return &sdkmcp.CallToolResult{Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: buf.String()}}}, nil
	}
}

// OKFValidateHandler is `ai-rulez okf validate <dir> --format json` for a bundle
// directory inside the project. A git URL is refused: tools never use the
// network. Findings at or above fail_on are an error result carrying the document.
func OKFValidateHandler(_ context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	spec := request.GetString("bundle", "")
	if spec == "" {
		return ToolError(oops.Errorf("bundle is required: the directory of the OKF bundle"))
	}
	if strings.Contains(spec, "://") || strings.HasPrefix(spec, "git@") || strings.HasPrefix(spec, "git+") {
		return ToolError(oops.Hint("clone it and pass the directory, or run `ai-rulez okf validate` on the command line").
			Errorf("okf_validate reads a local directory, not %q: tools never use the network", spec))
	}
	threshold, none, err := okf.ParseFailOn(request.GetString("fail_on", ""))
	if err != nil {
		return ToolError(err)
	}
	dir := spec
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(workingDir(request), dir)
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return ToolError(fmt.Errorf("%s is not a directory", spec))
	}
	b, err := okf.Load(os.DirFS(dir))
	if err != nil {
		return ToolError(err)
	}
	findings := okf.Check(b)
	return reportResult(okf.ValidationDocument(spec, b, findings), !none && okf.Fails(findings, threshold))
}

// ApprovalsStatusHandler is `ai-rulez approve --list --format json`: what needs
// approval and its status, read from the lock without fetching anything. It
// approves nothing: approving, revoking and signing stay on the command line.
func ApprovalsStatusHandler(version string) func(context.Context, *ToolRequest) (*sdkmcp.CallToolResult, error) {
	return func(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
		cfg, skipped, _, err := loadOffline(ctx, request, true)
		if err != nil {
			return ToolError(err)
		}
		lock, err := lockfile.Load(cfg.ConfigDir)
		if err != nil {
			return ToolError(err)
		}
		if lock == nil {
			return ToolError(oops.Hint("run `ai-rulez lock` first: only pinned content can be approved").
				Errorf("no %s in %s", lockfile.FileName, cfg.ConfigDir))
		}
		snap, err := govview.SnapshotContext(ctx, cfg, lock.Profile, true, version)
		if err != nil {
			return ToolError(err)
		}
		approvers := ""
		if cfg.Governance != nil {
			approvers = cfg.Governance.ApproversFrom
		}
		doc := govview.BuildApprovalList(govview.ApprovalListInput{
			Policy: approval.PolicyOfContext(ctx, cfg), Lock: lock, Subjects: approval.SubjectsOf(lock, snap.Items),
			Now: govview.ApprovalNow(), ApproversFrom: approvers, All: request.GetBool("all", false), Safe: govview.SafeText,
		})
		return governanceResult(doc, skipped)
	}
}

// PolicyViewer builds the report of the effective organization policy for the
// project at dir; cfg is nil when the project does not load. The command supplies
// the policy discovery of `validate --show-policy`.
type PolicyViewer func(ctx context.Context, dir string, cfg *config.Config) (policy.Report, error)

// PolicyShowWith returns the policy_show handler: `ai-rulez validate
// --show-policy --format json`. A policy the configuration tried to loosen (in
// enforce mode) is an error result carrying the report.
func PolicyShowWith(view PolicyViewer) func(context.Context, *ToolRequest) (*sdkmcp.CallToolResult, error) {
	return func(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
		if view == nil {
			return ToolError(oops.New("this server was built without policy discovery"))
		}
		base := workingDir(request)
		cfg, loadErr := loadProjectConfigWith(ctx, request, base, config.WithoutRemote())
		if loadErr != nil {
			cfg = nil // like the command: show the policy even where nothing loads
		}
		report, err := view(ctx, base, cfg)
		if err != nil {
			return ToolError(err)
		}
		return reportResult(report, report.Overrides.Rejected > 0 && report.Mode != policy.ModeWarn)
	}
}

// ListBuiltinsHandler is `ai-rulez builtins list --format json`.
func ListBuiltinsHandler(_ context.Context, _ *ToolRequest) (*sdkmcp.CallToolResult, error) {
	domains := builtins.List()
	items := make([]map[string]any, len(domains))
	for i, d := range domains {
		items[i] = map[string]any{
			keyName: d.Name, "category": string(d.Category), "auto_include": d.AutoInclude, "description": d.Description,
		}
	}
	return ToolSuccess(map[string]any{keyBuiltins: items, keyCount: len(items)})
}

// ListVerifiersHandler is `ai-rulez verifiers list --format json`: the declared
// verifiers, read from the configuration.
func ListVerifiersHandler(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	cfg, err := loadProjectConfigWith(ctx, request, workingDir(request), config.WithoutRemote())
	if err != nil {
		return ToolError(err)
	}
	if err := cfg.Validate(); err != nil {
		return ToolError(err)
	}
	rows := verifiers.List(cfg)
	return ToolSuccess(map[string]any{"verifiers": rows, keyCount: len(rows)})
}

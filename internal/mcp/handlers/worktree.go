package handlers

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/project"
	"github.com/Goldziher/ai-rulez/v5/internal/worktreeprovision"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/samber/oops"
)

// WorktreeSetupHandler provisions a linked worktree the agent's host or git
// already created: it regenerates the destination's outputs from the
// destination's own committed configuration and, with explicit consent,
// carries the source checkout's machine-local inputs. Branch creation stays
// with git or the CLI; this tool never creates a worktree or a branch.
//
// dry_run defaults to true when the caller omits it: a tool client that does
// not name the flag gets a plan, not a write. Only an explicit
// `dry_run: false` runs the real provisioning.
//
// The service refuses a destination that is not a registered linked worktree
// of the same repository, a copy target git tracks or does not ignore there,
// a conflicting destination file, and a destination whose configuration layout
// would strand the overlay. It reports paths, never file contents.
func WorktreeSetupHandler(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	source, err := filepath.Abs(workingDir(request))
	if err != nil {
		return ToolError(oops.Wrapf(err, "resolve the source checkout"))
	}
	destination := request.GetString("destination", "")
	if destination != "" && !filepath.IsAbs(destination) {
		// A relative destination names a path beside the source checkout, the
		// way the CLI resolves it from its working directory — not beside the
		// server process's cwd, which is not the caller's.
		destination = filepath.Join(source, destination)
	}
	dryRun, specified := request.arguments["dry_run"].(bool)
	if !specified {
		dryRun = true // omitted means plan: a write needs an explicit false
	}
	opts := &worktreeprovision.Options{
		Source:      source,
		Destination: destination,
		CopyLocal:   request.GetBool("copy_local", false),
		CopyExtra:   request.GetStringSlice("copy_extra", nil),
		DryRun:      dryRun,
		Generate:    generateWorktreeOutputs,
	}
	res, err := worktreeprovision.Setup(ctx, opts)
	if err != nil {
		return ToolError(err)
	}
	payload := map[string]any{
		keySuccess:    true,
		keyOperation:  "worktree_setup",
		keyMessage:    worktreeMessage(res),
		"source":      res.Source,
		"destination": res.Destination,
		"plan":        res.Plan,
	}
	if res.DryRun {
		payload["dry_run"] = true
	}
	if res.Generated {
		payload["generated"] = true
	}
	payload["copies"] = worktreeCopies(res.Copies)
	if len(res.Conflicts) > 0 {
		payload["conflicts"] = worktreeCopies(res.Conflicts)
	}
	return ToolSuccess(payload)
}

func worktreeCopies(entries []worktreeprovision.CopyEntry) []map[string]any {
	if len(entries) == 0 {
		return nil
	}
	out := make([]map[string]any, len(entries))
	for i, e := range entries {
		out[i] = map[string]any{"source": e.Source, "destination": e.Destination, "action": e.Action}
	}
	return out
}

// worktreeMessage is the one-line summary of what the run did or refused.
func worktreeMessage(res *worktreeprovision.Result) string {
	if len(res.Conflicts) > 0 {
		return fmt.Sprintf("Refused: %d destination file(s) differ from what the source would carry; nothing was written", len(res.Conflicts))
	}
	if res.DryRun {
		return fmt.Sprintf("Dry run: would provision %s (%d file(s) planned)", res.Destination, len(res.Copies))
	}
	return fmt.Sprintf("Provisioned %s (%d file(s) carried, outputs regenerated)", res.Destination, len(res.Copies))
}

// generateWorktreeOutputs regenerates the destination's outputs in process:
// the same load, validation and overwrite guard `generate_outputs` uses, but
// for the destination checkout and without this server's tool request (the
// destination has its own configuration and its own lock policy).
func generateWorktreeOutputs(ctx context.Context, projectDir string, dryRun bool) ([]string, error) {
	cfg, err := project.Load(ctx, projectDir,
		config.WithLockPolicy(config.LockPolicy{RequireWhenEnforced: true, Offline: true}))
	if err != nil {
		return nil, oops.Wrapf(err, "load the destination configuration")
	}
	if err := cfg.Validate(); err != nil {
		return nil, err //nolint:wrapcheck // already contextual
	}
	gen := generator.NewGenerator(cfg)
	gen.SetContext(ctx)
	if dryRun {
		plan, err := gen.DryRun("")
		if err != nil {
			return nil, oops.Wrapf(err, "plan the destination generate")
		}
		return plan, nil
	}
	if err := gen.Generate(""); err != nil {
		return nil, err //nolint:wrapcheck // already contextual
	}
	return nil, nil
}

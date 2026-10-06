// Package adapter holds the optimizers bundled with `ai-rulez improve`: small programs that speak
// optimizer protocol v1 (docs/improve.md) so the protocol has living examples. They run as a
// child process of improve like any other optimizer, inside the same sandbox, environment
// scrub, diff policy and held-out gate; nothing here is trusted by the gate.
package adapter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/improve"
)

// Prefix marks `--with builtin:<name>` and the [improve] optimizer value that name a bundled adapter.
const Prefix = "builtin:"

// Names of the bundled adapters.
const (
	// NoOp changes nothing; it exists so the protocol and the loop can be tested end to end.
	NoOp = "noop"
	// ReviewFix drives the review judge and fixer (`ai-rulez review fix`) on SKILL.md.
	ReviewFix = "review-fix"
	// Shell is a template, not a runnable adapter: `improve adapters shell` prints it.
	Shell = "shell"
	// Research is a documented recipe for an external research optimizer, not a runnable adapter.
	Research = "research"
)

// maxRequestBytes bounds the request an adapter reads from standard input.
const maxRequestBytes = 16 << 20

// Info describes one bundled adapter or template.
type Info struct {
	Name    string `json:"name"`
	Summary string `json:"summary"`
	// Runnable adapters are selected with --with builtin:<name>; the others are templates to copy.
	Runnable bool `json:"runnable"`
}

// List returns the bundled adapters and templates, sorted by name.
func List() []Info {
	out := []Info{
		{NoOp, "changes nothing and reports no cost; for tests and for trying the protocol", true},
		{ReviewFix, "judges SKILL.md with the review rubric and applies a verified fix (needs [llm] model, allow_network and a fixer model different from the judge)", true},
		{Shell, "a shell template that calls any tool on the workspace copy of the skill", false},
		{Research, "a recipe for wiring a research optimizer; its interface is unverified, so it is not shipped as supported", false},
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// IsRunnable reports whether name is a bundled adapter improve can run.
func IsRunnable(name string) bool {
	return name == NoOp || name == ReviewFix
}

// Name returns the adapter named by an optimizer value (builtin:<name>), and whether it is one.
func Name(optimizer string) (string, bool) {
	name, ok := strings.CutPrefix(strings.TrimSpace(optimizer), Prefix)
	return name, ok
}

// Options are the settings an adapter child receives on its command line.
type Options struct {
	ReviewFix ReviewFixOptions
}

// ReadRequest decodes the optimizer request from r, rejecting an oversized or malformed one.
func ReadRequest(r io.Reader) (*improve.OptimizerRequest, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxRequestBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read the request: %w", err)
	}
	if len(data) > maxRequestBytes {
		return nil, fmt.Errorf("the request is larger than %d bytes", maxRequestBytes)
	}
	var req improve.OptimizerRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("the request is not valid JSON: %w", err)
	}
	if req.Version != improve.ProtocolVersion {
		return nil, fmt.Errorf("the request is protocol version %d, want %d", req.Version, improve.ProtocolVersion)
	}
	return &req, nil
}

// Serve reads one request from in, runs the named adapter against the workspace (the directory
// holding <skill>/), and writes the response to out.
func Serve(ctx context.Context, name string, in io.Reader, out io.Writer, workspace string, opts *Options) error {
	if !IsRunnable(name) {
		return &RefusedError{Reason: fmt.Sprintf("%q is not a runnable adapter (runnable: %s, %s); `improve adapters %s` prints a template", name, NoOp, ReviewFix, name)}
	}
	req, err := ReadRequest(in)
	if err != nil {
		return err
	}
	var resp *improve.OptimizerResponse
	switch name {
	case NoOp:
		resp = &improve.OptimizerResponse{Version: improve.ProtocolVersion, Summary: "no-op adapter: made no change", Changed: []string{}}
	case ReviewFix:
		resp, err = RunReviewFix(ctx, req, workspace, &opts.ReviewFix)
		if err != nil {
			return err
		}
	}
	enc := json.NewEncoder(out)
	if err := enc.Encode(resp); err != nil {
		return fmt.Errorf("write the response: %w", err)
	}
	return nil
}

// RefusedError reports an adapter that cannot run (AR9J9): a missing model, no network opt-in, a
// fixer equal to the judge. It is reported on standard error and ends the child with a failure,
// which rejects the round.
type RefusedError struct{ Reason string }

func (e *RefusedError) Error() string { return improve.CodeAdapterRefused + ": " + e.Reason }

// IsRefused reports whether err is a RefusedError.
func IsRefused(err error) bool {
	var r *RefusedError
	return errors.As(err, &r)
}

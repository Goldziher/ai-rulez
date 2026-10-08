package commands

import (
	"context"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/policy"
	"github.com/Goldziher/ai-rulez/v5/internal/signing"
)

// runSignPolicy signs an organization policy file (--policy) with a key or
// keylessly and writes the bundle next to it as <file>.sigstore.json, the name
// the policy loader reads. It needs no project: a policy owner signs a file they
// publish. Exit codes: 0 signed, 1 the command could not run.
func runSignPolicy(ctx context.Context, env ambient.Env) int {
	signer, err := newSigner(ctx, env)
	if err != nil {
		renderStderr(err)
		return 1
	}
	if err := exportPublicKey(signer); err != nil {
		renderStderr(err)
		return 1
	}
	bundle, err := policy.SignPolicyFile(ctx, signer, signPolicy, time.Now())
	if err != nil {
		renderStderr(err)
		return 1
	}
	out := signOutput
	if out == "" {
		out = signPolicy + policy.SidecarSuffix
	}
	if out, err = appendTarget(out); err != nil {
		renderStderr(err)
		return 1
	}
	if err := writeBundle(out, bundle); err != nil {
		renderStderr(err)
		return 1
	}
	info, _ := signing.Inspect(bundle) //nolint:errcheck // display only
	reportSigned("Signed policy", map[string]any{"kind": "policy", "path": signPolicy, "signer": signerLabel(info), "bundle": out},
		"path", signPolicy, "signer", signerLabel(info), "bundle", out)
	return 0
}

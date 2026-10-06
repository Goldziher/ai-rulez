package commands

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"time"

	"github.com/samber/oops"
	"github.com/sigstore/sigstore-go/pkg/root"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/approval"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/govview"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/publish"
	"github.com/Goldziher/ai-rulez/v5/internal/sbom"
	"github.com/Goldziher/ai-rulez/v5/internal/signing"
)

// validatePublishSignFlags checks the signing flags of `publish`.
func validatePublishSignFlags() error {
	switch {
	case publishSignKey != "" && publishSignKeyless:
		return oops.Errorf("--sign-key and --sign-keyless are mutually exclusive")
	case publishSignKeyless && publishSignTLog:
		return oops.Errorf("--sign-tlog applies to --sign-key: keyless signatures are always logged")
	case !publishSignKeyless && (publishSignTokenEnv != "" || publishFulcioURL != "" || publishSignInteractive):
		return oops.Errorf("--sign-token-env, --sign-interactive and --fulcio-url apply to --sign-keyless")
	case publishSignKeyless && publishSignKeyPassEnv != "":
		return oops.Errorf("--sign-key-password-env applies to --sign-key")
	case publishRekorURL != "" && !publishSignKeyless && !publishSignTLog:
		return oops.Errorf("--rekor-url applies to --sign-keyless or --sign-tlog: without a log the signature is never sent anywhere")
	}
	if err := checkSigstoreURL("--fulcio-url", publishFulcioURL); err != nil {
		return err
	}
	return checkSigstoreURL("--rekor-url", publishRekorURL)
}

func publishSigning() bool { return publishSignKey != "" || publishSignKeyless }

// publishSigner builds the signer the flags ask for; nil when not signing.
func publishSigner(ctx context.Context, env ambient.Env) (signing.Signer, error) {
	switch {
	case publishSignKeyless:
		fmt.Fprintln(os.Stderr, "keyless signing: your OIDC identity, the certificate and the archive digest go to a transparency log that is public unless --rekor-url names another one")
		tok, err := signing.ResolveIDToken(ctx, env, publishSignTokenEnv, publishSignInteractive)
		if err != nil {
			return nil, err //nolint:wrapcheck // already contextual
		}
		return signing.NewKeylessSigner(signing.KeylessOptions{IDToken: tok, FulcioURL: publishFulcioURL, RekorURL: publishRekorURL})
	case publishSignKey != "":
		data, err := readKeyFile(publishSignKey)
		if err != nil {
			return nil, err
		}
		ks, err := signing.LoadKeySigner(data, []byte(publishKeyPassword(env)))
		if err != nil {
			return nil, oops.With("path", publishSignKey).Wrap(err)
		}
		ks.TLog, ks.RekorURL = publishSignTLog, publishRekorURL
		return ks, nil
	}
	return nil, nil //nolint:nilnil // not signing
}

func publishKeyPassword(env ambient.Env) string {
	if publishSignKeyPassEnv != "" {
		return ambient.Getenv(env, publishSignKeyPassEnv)
	}
	if p := ambient.Getenv(env, signPasswordEnv); p != "" {
		return p
	}
	return ambient.Getenv(env, signCosignPasswordEnv)
}

// signFunc turns a signer into the callback Build calls with the archive.
func signFunc(ctx context.Context, s signing.Signer) func([]byte) (*publish.SignResult, error) {
	if s == nil {
		return nil
	}
	return func(archive []byte) (*publish.SignResult, error) { return publish.SignArchive(ctx, s, archive) }
}

// approvalGate reads the approval state of the lock. It returns the summary the
// manifest records when the [governance] policy selects content, and fails
// (AR9N8) when require is set and anything selected lacks a valid approval.
func approvalGate(cfg *config.Config, require bool) (*publish.ApprovalInfo, error) {
	lock, err := lockfile.Load(cfg.ConfigDir)
	if err != nil || lock == nil {
		return nil, publish.Errorf(publish.CodePreflight, publish.ExitGate, "run `ai-rulez lock`", "cannot read %s", lockfile.FileName)
	}
	policy := approval.PolicyOf(cfg)
	if !policy.Active() {
		if require {
			return nil, publish.Errorf(publish.CodeUnapproved, publish.ExitGate,
				"select content in [governance] (see docs/approvals.md) or drop require_approved",
				"require_approved is set but the [governance] policy selects nothing to approve")
		}
		return nil, nil //nolint:nilnil // no policy, nothing to record
	}
	if msg := policy.LockProblem(lock); msg != "" {
		return nil, publish.Errorf(publish.CodeUnapproved, publish.ExitGate, "run `ai-rulez lock`", "%s", msg)
	}
	snap, err := lockSnapshot(cfg, lock.Profile, true)
	if err != nil {
		return nil, err
	}
	st := govview.EvaluateApprovals(cfg, lock, snap.Items, govview.ApprovalNow())
	info := &publish.ApprovalInfo{}
	var failing []string
	for i := range st.Results {
		r := &st.Results[i]
		if !r.Required {
			continue
		}
		info.Required++
		if r.Status == approval.StatusOK {
			info.Approved++
			continue
		}
		failing = append(failing, fmt.Sprintf("%s %s (%s)", r.Kind, r.ID, r.Status))
	}
	if require && len(failing) > 0 {
		shown := failing
		if len(shown) > maxPublishListed {
			shown = append(append([]string(nil), shown[:maxPublishListed]...), fmt.Sprintf("and %d more", len(failing)-maxPublishListed))
		}
		return nil, publish.Errorf(publish.CodeUnapproved, publish.ExitGate, "review with `ai-rulez approve --list`, then `ai-rulez approve`",
			"%d of %d items the governance policy selects lack a valid approval: %v", len(failing), info.Required, shown)
	}
	return info, nil
}

// maxPublishListed bounds how many unapproved items an error names.
const maxPublishListed = 10

// publishSBOM renders the project SBOM (CycloneDX) from the lock and the cache.
func publishSBOM(cfg *config.Config) ([]byte, error) {
	bom, err := sbom.Build(cfg, Version)
	if err != nil {
		return nil, err //nolint:wrapcheck // already contextual
	}
	var buf bytes.Buffer
	if err := sbom.Write(&buf, bom); err != nil {
		return nil, err //nolint:wrapcheck // already contextual
	}
	return buf.Bytes(), nil
}

// publishVerifyOptions builds the trust for `publish verify` from its flags.
func publishVerifyOptions(env ambient.Env) (publish.VerifyOptions, error) {
	o := publish.VerifyOptions{Now: time.Now()}
	for _, path := range publishVerifyKeys {
		data, err := readKeyFile(path)
		if err != nil {
			return o, err
		}
		key, err := signing.ParsePublicKey(data)
		if err != nil {
			return o, oops.With("path", path).Wrap(err)
		}
		o.Keys = append(o.Keys, key)
	}
	if (publishVerifyIdentity == "") != (publishVerifyIssuer == "") {
		return o, oops.Errorf("--identity and --issuer go together")
	}
	if publishVerifyIdentity != "" {
		o.Identities = append(o.Identities, signing.TrustEntry{Identity: publishVerifyIdentity, Issuer: publishVerifyIssuer})
	}
	if publishVerifyRoot != "" {
		data, err := readKeyFile(publishVerifyRoot)
		if err != nil {
			return o, err
		}
		tr, err := root.NewTrustedRootFromJSON(data)
		if err != nil {
			return o, oops.With("path", publishVerifyRoot).Wrapf(err, "the trusted root is not valid")
		}
		o.TrustedRoot = tr
	} else if len(o.Identities) > 0 {
		if tr := cachedTrustedRoot(env); tr != nil {
			o.TrustedRoot = tr
		}
	}
	return o, nil
}

// cachedTrustedRoot reads the root `ai-rulez trust update` cached, or nil.
func cachedTrustedRoot(env ambient.Env) root.TrustedMaterial {
	dir, err := config.CacheDirIn(env, "sigstore")
	if err != nil {
		return nil
	}
	data, err := readKeyFile(dir + string(os.PathSeparator) + signing.TrustedRootFile)
	if err != nil {
		return nil
	}
	tr, err := root.NewTrustedRootFromJSON(data)
	if err != nil {
		return nil
	}
	return tr
}

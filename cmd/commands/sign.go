package commands

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/samber/oops"
	"github.com/spf13/cobra"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
	"github.com/Goldziher/ai-rulez/v5/internal/signing"
)

const (
	// signPasswordEnv is the default variable holding a signing key's password;
	// COSIGN_PASSWORD is read when it is unset, so a cosign key works as it is.
	signPasswordEnv       = "AI_RULEZ_SIGNING_KEY_PASSWORD"
	signCosignPasswordEnv = "COSIGN_PASSWORD"
	gitProbeTimeout       = 5 * time.Second
	signedFileMode        = 0o644
)

var (
	signLock        bool
	signKey         string
	signKeyPassEnv  string
	signKeyless     bool
	signTokenEnv    string
	signFulcioURL   string
	signRekorURL    string
	signTLog        bool
	signEmbedItems  bool
	signOutput      string
	signInteractive bool
)

// SignCmd signs the lock-subject statement into a Sigstore bundle.
var SignCmd = &cobra.Command{
	Use:   "sign [config-file]",
	Short: "Sign the lock into a Sigstore bundle (DSSE over an in-toto statement)",
	Long: `Sign the lock-subject statement of ai-rulez.lock (see "lock --subject") and write a
Sigstore bundle next to the lock (.ai-rulez/ai-rulez.lock.sigstore.json). The
bundle holds a DSSE envelope over an in-toto statement whose subject is the
lock-subject digest, so it survives re-formatting of the TOML and verifies with
"ai-rulez verify --attestation" or cosign verify-blob-attestation.

  ai-rulez sign --lock --key cosign.key      sign with a key (offline)
  ai-rulez sign --lock --keyless             sign with a Fulcio certificate and a
                                             Rekor log entry (network, opt-in)

Key mode reads a PEM private key: ECDSA P-256/P-384/P-521 or ed25519, PKCS#8 or a
cosign key. An encrypted key's password comes from AI_RULEZ_SIGNING_KEY_PASSWORD
or COSIGN_PASSWORD (--key-password-env names another variable); it is never a flag.

Keyless mode sends the OIDC token to the certificate authority and the signature,
the certificate (which names your identity), the repository claim and the subject
digest to a transparency log that is public unless --rekor-url points elsewhere.
The token comes from the variable named by --identity-token-env, else the GitHub
Actions runtime (needs permissions: id-token: write), else --interactive opens a
browser. Use key mode for a private repository you do not want logged.

Run it after the final "ai-rulez lock": any change to the lock invalidates the
signature. Exit codes: 0 signed, 1 the command could not run, 2 the lock is
stale (its tree does not match its entries).`,
	Args: cobra.MaximumNArgs(1),
	Run: func(_ *cobra.Command, args []string) {
		if code := runSign(context.Background(), args, nil); code != 0 {
			os.Exit(code)
		}
	},
}

func init() {
	f := SignCmd.Flags()
	f.BoolVar(&signLock, "lock", false, "Sign the lock-subject statement of ai-rulez.lock")
	f.StringVar(&signKey, "key", "", "PEM private key to sign with (ECDSA or ed25519; cosign keys work)")
	f.StringVar(&signKeyPassEnv, "key-password-env", "", "Environment variable holding the key password (default AI_RULEZ_SIGNING_KEY_PASSWORD, then COSIGN_PASSWORD)")
	f.BoolVar(&signKeyless, "keyless", false, "Sign with a short-lived Fulcio certificate and log the signature in Rekor (network; public log)")
	f.StringVar(&signTokenEnv, "identity-token-env", "", "With --keyless: environment variable holding the OIDC token (default: the GitHub Actions runtime token)")
	f.BoolVar(&signInteractive, "interactive", false, "With --keyless: open a browser for the OIDC login when no token is available")
	f.StringVar(&signFulcioURL, "fulcio-url", "", "With --keyless: Fulcio URL (default "+signing.DefaultFulcioURL+")")
	f.StringVar(&signRekorURL, "rekor-url", "", "Rekor URL for --keyless or --tlog (default "+signing.DefaultRekorURL+")")
	f.BoolVar(&signTLog, "tlog", false, "With --key: also record the signature in the Rekor transparency log (network; public log)")
	f.BoolVar(&signEmbedItems, "embed-items", false, "Put the pinned item ids and digests in the statement (ids can be sensitive in a private repository)")
	f.StringVar(&signOutput, "output", "", "Write the bundle here instead of next to the lock")
	f.StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
}

func validateSignFlags() error {
	switch {
	case !signLock:
		return oops.Hint("pass --lock").Errorf("nothing to sign")
	case signKey == "" && !signKeyless:
		return oops.Hint("pass --key <file> for key mode, or --keyless").Errorf("choose how to sign")
	case signKey != "" && signKeyless:
		return oops.Errorf("--key and --keyless are mutually exclusive")
	case signKeyless && signTLog:
		return oops.Errorf("--tlog applies to --key: keyless signatures are always logged")
	case !signKeyless && (signTokenEnv != "" || signFulcioURL != "" || signInteractive):
		return oops.Errorf("--identity-token-env, --interactive and --fulcio-url apply to --keyless")
	case signKeyless && signKeyPassEnv != "":
		return oops.Errorf("--key-password-env applies to --key")
	}
	return nil
}

// runSign signs the lock of the project at args[0] (or the current directory)
// and returns the exit code.
func runSign(ctx context.Context, args []string, env ambient.Env) int {
	if err := validateSignFlags(); err != nil {
		fmtError(err)
		return 1
	}
	path := ""
	if len(args) > 0 {
		path = args[0]
	}
	cfg, _, err := loadForLockCheck(path)
	if err != nil {
		fmtError(err)
		return 1
	}
	lock, err := lockfile.Load(cfg.ConfigDir)
	if err != nil {
		fmtError(err)
		return 1
	}
	meta := signing.LockMeta{Version: Version, Now: time.Now(), EmbedItems: signEmbedItems}
	meta.Repository, meta.Ref = detectRepo(ctx, cfg.BaseDir, env)
	signer, err := newSigner(ctx, env)
	if err != nil {
		fmtError(err)
		return 1
	}
	bundle, err := signing.SignLock(ctx, signer, lock, meta)
	if err != nil {
		fmtError(err)
		if signing.CodeOf(err) == signing.CodeSubjectMismatch {
			return exitDrift
		}
		return 1
	}
	out := signOutput
	if out == "" {
		out = filepath.Join(cfg.ConfigDir, filepath.FromSlash(attestationName(cfg)))
	}
	if err := writeBundle(out, bundle); err != nil {
		fmtError(err)
		return 1
	}
	info, _ := signing.Inspect(bundle) //nolint:errcheck // display only
	subject := signingSubject(lock)
	logger.Success("Signed "+lockfile.FileName, "signer", signerLabel(info), "subject", subject, "bundle", out)
	return 0
}

func attestationName(cfg *config.Config) string {
	if cfg.Signing != nil && cfg.Signing.Attestation != "" {
		return cfg.Signing.Attestation
	}
	return config.SigningAttestationFile
}

func signingSubject(lock *lockfile.File) string {
	st, err := signing.LockStatement(lock, signing.LockMeta{})
	if err != nil || len(st.Subject) == 0 {
		return ""
	}
	return "sha256:" + st.Subject[0].Digest["sha256"]
}

func signerLabel(info signing.SignerInfo) string {
	if info.Kind == signing.KindKeyless {
		return fmt.Sprintf("%s (issuer %s)", info.Identity, info.Issuer)
	}
	return "key " + info.KeyID
}

func writeBundle(path string, data []byte) error {
	if err := safefs.EnsureParent(path); err != nil {
		return oops.Wrap(err)
	}
	if err := safefs.WriteFileAtomic(path, data); err != nil {
		return oops.Wrap(err)
	}
	// The bundle is public (it is committed); WriteFileAtomic writes 0600.
	if err := os.Chmod(path, signedFileMode); err != nil { //nolint:gosec // a public attestation
		return oops.With("path", path).Wrapf(err, "set the bundle mode")
	}
	return nil
}

func newSigner(ctx context.Context, env ambient.Env) (signing.Signer, error) {
	if signKeyless {
		fmt.Fprintln(os.Stderr, "keyless signing: your OIDC identity, the certificate and the subject digest go to a transparency log that is public unless --rekor-url names another one")
		tok, err := signing.ResolveIDToken(ctx, env, signTokenEnv, signInteractive)
		if err != nil {
			return nil, err //nolint:wrapcheck // already contextual
		}
		return signing.NewKeylessSigner(signing.KeylessOptions{IDToken: tok, FulcioURL: signFulcioURL, RekorURL: signRekorURL})
	}
	data, err := os.ReadFile(signKey) //nolint:gosec // the user names their own key file
	if err != nil {
		return nil, oops.With("path", signKey).Wrapf(err, "read the signing key")
	}
	if len(data) > 1<<20 {
		return nil, oops.With("path", signKey).Errorf("the signing key file is too large")
	}
	ks, err := signing.LoadKeySigner(data, []byte(keyPassword(env)))
	if err != nil {
		return nil, oops.With("path", signKey).Wrap(err)
	}
	ks.TLog, ks.RekorURL = signTLog, signRekorURL
	return ks, nil
}

func keyPassword(env ambient.Env) string {
	if signKeyPassEnv != "" {
		return ambient.Getenv(env, signKeyPassEnv)
	}
	if p := ambient.Getenv(env, signPasswordEnv); p != "" {
		return p
	}
	return ambient.Getenv(env, signCosignPasswordEnv)
}

// detectRepo returns the repository URL and ref claims for the statement,
// best effort: the GitHub Actions variables, else the git origin and HEAD. A
// remote URL loses its credentials.
func detectRepo(ctx context.Context, dir string, env ambient.Env) (repo, ref string) {
	if gh := ambient.Getenv(env, "GITHUB_REPOSITORY"); gh != "" {
		server := ambient.Getenv(env, "GITHUB_SERVER_URL")
		if server == "" {
			server = "https://github.com"
		}
		return strings.TrimSuffix(server, "/") + "/" + gh, ambient.Getenv(env, "GITHUB_REF")
	}
	repo = normalizeRemote(gitOutput(ctx, dir, "remote", "get-url", "origin"))
	ref = gitOutput(ctx, dir, "symbolic-ref", "-q", "HEAD")
	return repo, ref
}

func gitOutput(ctx context.Context, dir string, args ...string) string {
	ctx, cancel := context.WithTimeout(ctx, gitProbeTimeout)
	defer cancel()
	out, err := gitutil.Command(ctx, dir, args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// normalizeRemote turns a git remote into an https URL without credentials or a
// .git suffix, or "" when it has no recognizable host and path.
func normalizeRemote(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		// scp-like: git@github.com:org/repo.git
		at := strings.LastIndex(raw, "@")
		host, path, ok := strings.Cut(raw[at+1:], ":")
		if !ok || host == "" || path == "" {
			return ""
		}
		return "https://" + host + "/" + strings.TrimSuffix(strings.TrimPrefix(path, "/"), ".git")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	return "https://" + u.Hostname() + strings.TrimSuffix(u.Path, ".git")
}

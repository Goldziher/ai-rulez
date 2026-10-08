package commands

import (
	"context"
	"fmt"
	"io"
	"net"
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
	"github.com/Goldziher/ai-rulez/v5/internal/jsondoc"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
	"github.com/Goldziher/ai-rulez/v5/internal/signing"
	"github.com/Goldziher/ai-rulez/v5/internal/signing/sigstore"
)

const (
	// signPasswordEnv is the default variable holding a signing key's password;
	// COSIGN_PASSWORD is read when it is unset, so a cosign key works as it is.
	signPasswordEnv       = "AI_RULEZ_SIGNING_KEY_PASSWORD"
	signCosignPasswordEnv = "COSIGN_PASSWORD"
	gitProbeTimeout       = 5 * time.Second
	signedFileMode        = 0o644
	maxSigningKeyBytes    = 1 << 20
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
	signBundle      string
	signSkill       string
	signSBOM        string
	signPolicy      string
	signProvenance  bool
	signBuilderID   string
	signAppend      bool
	signPublicOut   string
)

// SignCmd signs the lock-subject statement into a Sigstore bundle.
var SignCmd = &cobra.Command{
	Use:   "sign [config-file]",
	Short: "Sign the lock, a plugin bundle, a skill or an SBOM into a Sigstore bundle",
	Long: `Sign one subject into a Sigstore bundle: a DSSE envelope over an in-toto statement.

  ai-rulez sign --lock                 the lock-subject statement of ai-rulez.lock (see
                                       "lock --subject"), written next to the lock
                                       (.ai-rulez/ai-rulez.lock.sigstore.json)
  ai-rulez sign --bundle <dir>         a generated plugin bundle: the tree digest of every
                                       file in the directory, written to
                                       <dir>/.ai-rulez.sigstore.json; --provenance also
                                       writes a SLSA v1 provenance statement beside it
  ai-rulez sign --skill <dir>          a skill directory a publisher ships, written to
                                       <dir>/.ai-rulez.sigstore.json; consumers verify it
                                       against [[signing.trust]] entries with subject = "skill"
  ai-rulez sign --sbom <file>          any SBOM file (ai-rulez sbom, SPDX, CycloneDX),
                                       written to <file>.sigstore.json
  ai-rulez sign --policy <file>        an organization policy file, written to
                                       <file>.sigstore.json, where the policy loader looks
                                       (--policy-signer-key / --policy-signer-identity, see
                                       docs/policy.md); it is refused unless the file parses

The lock statement survives re-formatting of the TOML. All of them verify with
"ai-rulez verify --attestation" (--bundle, --skill, --sbom) or cosign
verify-blob-attestation.

  ai-rulez sign --lock --key cosign.key      sign with a key (offline)
  ai-rulez sign --lock --key awskms:///alias/release
                                             sign with a key held in a KMS (awskms://,
                                             gcpkms://, azurekms://, hashivault://)
  ai-rulez sign --lock --keyless             sign with a Fulcio certificate and a
                                             Rekor log entry (network, opt-in)
  ai-rulez sign --lock --key other.key --append
                                             add a second signer's file
                                             (ai-rulez.lock.2.sigstore.json) for
                                             [signing] thresholds

Key mode reads a PEM private key: ECDSA P-256/P-384/P-521 or ed25519, PKCS#8 or a
cosign key. An encrypted key's password comes from AI_RULEZ_SIGNING_KEY_PASSWORD
or COSIGN_PASSWORD (--key-password-env names another variable); it is never a flag.
A KMS key never leaves the KMS: the provider's usual credentials (AWS_*,
GOOGLE_APPLICATION_CREDENTIALS, AZURE_*, VAULT_*) are read from the environment.
--public-key-out writes the key's PEM public key, to trust it in [signing].

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
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmdContext()
		if outFor(cmd).JSON() {
			ctx = withSignRecorder(ctx)
		}
		return exitStatus(runSign(ctx, args, nil))
	},
}

// githubServerURL is the GITHUB_SERVER_URL of github.com, the default when the
// Actions environment does not set one.
const githubServerURL = "https://github.com"

func init() {
	f := SignCmd.Flags()
	f.BoolVar(&signLock, "lock", false, "Sign the lock-subject statement of ai-rulez.lock")
	f.StringVar(&signBundle, "bundle", "", "Sign the plugin bundle directory (tree digest of its files) into <dir>/.ai-rulez.sigstore.json")
	f.StringVar(&signSkill, "skill", "", "Sign a skill directory a publisher ships into <dir>/.ai-rulez.sigstore.json")
	f.StringVar(&signSBOM, "sbom", "", "Sign an SBOM file into <file>.sigstore.json")
	f.StringVar(&signPolicy, "policy", "", "Sign an organization policy file into <file>.sigstore.json")
	f.BoolVar(&signProvenance, "provenance", false, "With --bundle: also write a SLSA v1 provenance statement (.ai-rulez.provenance.sigstore.json); it is the signer's own account of the build")
	f.StringVar(&signBuilderID, "builder-id", "", "With --provenance: the builder id to record (default: the GitHub Actions workflow reference, else ai-rulez's own)")
	f.BoolVar(&signAppend, "append", false, "Write a co-signature file next to the existing attestation instead of replacing it (for [signing] thresholds)")
	f.StringVar(&signPublicOut, "public-key-out", "", "With --key: write the signing key's PEM public key to this file")
	f.StringVar(&signKey, "key", "", "PEM private key file, or a KMS key URI (awskms://, gcpkms://, azurekms://, hashivault://), to sign with")
	f.StringVar(&signKeyPassEnv, "key-password-env", "", "Environment variable holding the key password (default AI_RULEZ_SIGNING_KEY_PASSWORD, then COSIGN_PASSWORD)")
	f.BoolVar(&signKeyless, "keyless", false, "Sign with a short-lived Fulcio certificate and log the signature in Rekor (network; public log)")
	f.StringVar(&signTokenEnv, "identity-token-env", "", "With --keyless: environment variable holding the OIDC token (default: the GitHub Actions runtime token)")
	f.BoolVar(&signInteractive, "interactive", false, "With --keyless: open a browser for the OIDC login when no token is available")
	f.StringVar(&signFulcioURL, "fulcio-url", "", "With --keyless: Fulcio URL (default "+sigstore.DefaultFulcioURL+")")
	f.StringVar(&signRekorURL, "rekor-url", "", "Rekor URL for --keyless or --tlog (default "+sigstore.DefaultRekorURL+")")
	f.BoolVar(&signTLog, "tlog", false, "With --key: also record the signature in the Rekor transparency log (network; public log)")
	f.BoolVar(&signEmbedItems, "embed-items", false, "Put the pinned item ids and digests in the statement (ids can be sensitive in a private repository)")
	f.StringVar(&signOutput, "output", "", "Write the bundle here instead of next to the lock")
	f.StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
	addFormatFlag(f, new(string), formatText, formatText, formatText, formatJSON)
}

func validateSignFlags() error {
	if err := validateSignSubjectFlags(); err != nil {
		return err
	}
	return validateSignModeFlags()
}

// validateSignSubjectFlags checks that one subject is chosen and its flags fit it.
func validateSignSubjectFlags() error {
	subjects := 0
	for _, set := range []bool{signLock, signBundle != "", signSkill != "", signSBOM != "", signPolicy != ""} {
		if set {
			subjects++
		}
	}
	switch {
	case subjects == 0:
		return oops.Hint("pass --lock, --bundle <dir>, --skill <dir>, --sbom <file> or --policy <file>").Errorf("nothing to sign")
	case subjects > 1:
		return oops.Errorf("--lock, --bundle, --skill, --sbom and --policy are mutually exclusive: sign one subject at a time")
	case signProvenance && signBundle == "":
		return oops.Errorf("--provenance applies to --bundle")
	case signProvenance && signAppend:
		return oops.Errorf("--provenance and --append do not combine: provenance is one statement by the builder")
	case signBuilderID != "" && !signProvenance:
		return oops.Errorf("--builder-id applies to --provenance")
	case signEmbedItems && !signLock:
		return oops.Errorf("--embed-items applies to --lock")
	case signPublicOut != "" && signKey == "":
		return oops.Errorf("--public-key-out applies to --key")
	}
	return nil
}

// validateSignModeFlags checks the key or keyless flags and the Sigstore URLs.
func validateSignModeFlags() error {
	switch {
	case signKey == "" && !signKeyless:
		return oops.Hint("pass --key <file> for key mode, or --keyless").Errorf("choose how to sign")
	case signKey != "" && signKeyless:
		return oops.Errorf("--key and --keyless are mutually exclusive")
	case signKeyless && signTLog:
		return oops.Errorf("--tlog applies to --key: keyless signatures are always logged")
	}
	return validateSignKeylessFlags()
}

// validateSignKeylessFlags checks the flags that belong to one signing mode,
// and the Sigstore URLs.
func validateSignKeylessFlags() error {
	switch {
	case !signKeyless && (signTokenEnv != "" || signFulcioURL != "" || signInteractive):
		return oops.Errorf("--identity-token-env, --interactive and --fulcio-url apply to --keyless")
	case signKeyless && signKeyPassEnv != "":
		return oops.Errorf("--key-password-env applies to --key")
	case signRekorURL != "" && !signKeyless && !signTLog:
		return oops.Errorf("--rekor-url applies to --keyless or --tlog: without a log the signature is never sent anywhere")
	}
	if err := checkSigstoreURL("--fulcio-url", signFulcioURL); err != nil {
		return err
	}
	return checkSigstoreURL("--rekor-url", signRekorURL)
}

// checkSigstoreURL accepts an https URL, or http only to a loopback host (a local
// Sigstore stack): the OIDC token and the signature must not cross the network
// in clear text.
func checkSigstoreURL(flag, raw string) error {
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return oops.Errorf("%s %q is not a URL with a host", flag, raw)
	}
	if u.Scheme == "https" {
		return nil
	}
	if ip := net.ParseIP(u.Hostname()); u.Scheme == "http" && (u.Hostname() == "localhost" || (ip != nil && ip.IsLoopback())) {
		return nil
	}
	return oops.Hint("use an https:// URL").Errorf("%s %q must be https (plain http is allowed only for localhost)", flag, raw)
}

// loadSignLock loads the project at args[0] (or the current directory) and its lock.
func loadSignLock(ctx context.Context, args []string) (*config.Config, *lockfile.File, error) {
	path := ""
	if len(args) > 0 {
		path = args[0]
	}
	cfg, _, err := loadForLockCheckContext(ctx, path)
	if err != nil {
		return nil, nil, err
	}
	lock, err := lockfile.Load(cfg.ConfigDir)
	if err != nil {
		return nil, nil, err
	}
	return cfg, lock, nil
}

// runSign signs the lock of the project at args[0] (or the current directory)
// and returns the exit code.
func runSign(ctx context.Context, args []string, env ambient.Env) int {
	code := signOne(ctx, args, env)
	if rec := signRecorderFrom(ctx); rec != nil && code == 0 {
		if err := jsondoc.Write(os.Stdout, map[string]any{"status": "signed", "signed": rec.entries}); err != nil {
			renderStderr(err)
			return exitFailure
		}
	}
	return code
}

// signRecorder collects what one run signed, for the --format json document. It
// travels in the run's context: its presence means --format json.
type signRecorder struct{ entries []map[string]any }

type signRecorderKey struct{}

func withSignRecorder(ctx context.Context) context.Context {
	return context.WithValue(ctx, signRecorderKey{}, &signRecorder{})
}

func signRecorderFrom(ctx context.Context) *signRecorder {
	rec, _ := ctx.Value(signRecorderKey{}).(*signRecorder) //nolint:errcheck // absent means text output
	return rec
}

// reportSigned announces one signed subject: a success line on stderr, or with
// --format json an entry of the document runSign prints on stdout.
func reportSigned(ctx context.Context, msg string, entry map[string]any, logArgs ...any) {
	if rec := signRecorderFrom(ctx); rec != nil {
		rec.entries = append(rec.entries, entry)
		return
	}
	logger.Success(msg, logArgs...)
}

func signOne(ctx context.Context, args []string, env ambient.Env) int {
	if err := validateSignFlags(); err != nil {
		renderStderr(err)
		return 1
	}
	if signPolicy != "" {
		return runSignPolicy(ctx, env)
	}
	if signBundle != "" || signSkill != "" || signSBOM != "" {
		return runSignArtifact(ctx, env)
	}
	cfg, lock, err := loadSignLock(ctx, args)
	if err != nil {
		renderStderr(err)
		return 1
	}
	meta := signing.LockMeta{Version: Version, Now: time.Now(), EmbedItems: signEmbedItems}
	meta.Repository, meta.Ref = detectRepo(ctx, cfg.BaseDir, env)
	signer, err := newSigner(ctx, env)
	if err != nil {
		renderStderr(err)
		return 1
	}
	if err := exportPublicKey(signer); err != nil {
		renderStderr(err)
		return 1
	}
	bundle, err := signing.SignLock(ctx, signer, lock, meta)
	if err != nil {
		renderStderr(err)
		if signing.CodeOf(err) == signing.CodeSubjectMismatch {
			return exitDrift
		}
		return 1
	}
	out := signOutput
	if out == "" {
		out = filepath.Join(cfg.ConfigDir, filepath.FromSlash(attestationName(cfg)))
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
	subject := signingSubject(lock)
	reportSigned(ctx, "Signed "+lockfile.FileName, map[string]any{"kind": "lock", "path": lockfile.FileName, "signer": signerLabel(info), "subject": subject, "bundle": out},
		"signer", signerLabel(info), "subject", subject, "bundle", out)
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
		tok, err := sigstore.ResolveIDToken(ctx, env, signTokenEnv, signInteractive)
		if err != nil {
			return nil, err //nolint:wrapcheck // already contextual
		}
		return sigstore.NewKeylessSigner(sigstore.KeylessOptions{IDToken: tok, FulcioURL: signFulcioURL, RekorURL: signRekorURL})
	}
	var ks *sigstore.KeySigner
	if sigstore.IsKMSRef(signKey) {
		var err error
		if ks, err = sigstore.LoadKMSSigner(ctx, signKey); err != nil {
			return nil, err //nolint:wrapcheck // already contextual
		}
	} else {
		data, err := readKeyFile(signKey)
		if err != nil {
			return nil, err
		}
		if ks, err = sigstore.LoadKeySigner(data, []byte(keyPassword(env))); err != nil {
			return nil, oops.With("path", signKey).Wrap(err)
		}
	}
	ks.TLog, ks.RekorURL = signTLog, signRekorURL
	return ks, nil
}

// exportPublicKey writes the PEM public key of a --key signer to --public-key-out,
// so a KMS key can be named in [signing] without another tool.
func exportPublicKey(s signing.Signer) error {
	if signPublicOut == "" {
		return nil
	}
	ks, ok := s.(*sigstore.KeySigner)
	if !ok {
		return oops.Errorf("--public-key-out applies to --key")
	}
	pemText, err := ks.Key.GetPublicKeyPem()
	if err != nil {
		return oops.Wrapf(err, "encode the public key")
	}
	return writeBundle(signPublicOut, []byte(pemText))
}

// appendTarget turns the attestation path into the first free co-signature file
// when --append is set; the primary must exist.
func appendTarget(out string) (string, error) {
	if !signAppend {
		return out, nil
	}
	if _, err := os.Stat(out); err != nil {
		return "", oops.Hint("sign without --append first").Errorf("--append adds a co-signature to an existing attestation, and %s does not exist", out)
	}
	return signing.NextCosignaturePath(out), nil
}

// readKeyFile reads a signing key, refusing a file over maxSigningKeyBytes
// before it is read whole.
func readKeyFile(path string) ([]byte, error) {
	f, err := os.Open(path) //nolint:gosec // the user names their own key file
	if err != nil {
		return nil, oops.With("path", path).Wrapf(err, "read the signing key")
	}
	defer f.Close() //nolint:errcheck // read-only
	data, err := io.ReadAll(io.LimitReader(f, maxSigningKeyBytes+1))
	if err != nil {
		return nil, oops.With("path", path).Wrapf(err, "read the signing key")
	}
	if len(data) > maxSigningKeyBytes {
		return nil, oops.With("path", path).Errorf("the signing key file is too large")
	}
	return data, nil
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
			server = githubServerURL
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

// isWindowsDrivePath reports whether raw starts with a drive letter and a path
// separator ("C:/x", `C:\x`), which would otherwise parse as host "C".
func isWindowsDrivePath(raw string) bool {
	if len(raw) < 3 || raw[1] != ':' || (raw[2] != '/' && raw[2] != '\\') {
		return false
	}
	c := raw[0] | 0x20
	return c >= 'a' && c <= 'z'
}

// normalizeRemote turns a git remote into an https URL without credentials or a
// .git suffix, or "" when it has no recognizable host and path.
func normalizeRemote(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		if isWindowsDrivePath(raw) {
			return "" // a local path, not an scp-like remote
		}
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

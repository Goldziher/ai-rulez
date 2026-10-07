package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/samber/oops"
	"github.com/spf13/cobra"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/sbom"
)

const (
	formatCycloneDX = sbom.FormatCycloneDX
	// sourceDateEpochEnv is the reproducible-builds variable that fixes the document time.
	sourceDateEpochEnv = "SOURCE_DATE_EPOCH"
	sbomFileMode       = 0o644
)

// sbomFlags are the flags of `ai-rulez sbom`.
type sbomFlags struct {
	format, output, files, profile, role, timestamp string
	online, includeOutputs, noApprovals             bool
	redactReviewers, verify                         bool
	requireLock, strictPins, check                  bool
}

var sbomOpts sbomFlags

// SBOMCmd prints the project's AI configuration as a CycloneDX or SPDX bill of materials.
var SBOMCmd = &cobra.Command{
	Use:   "sbom",
	Short: "Print a CycloneDX or SPDX software bill of materials of the AI configuration",
	Long: `Print a bill of materials of the project's AI configuration, as CycloneDX 1.6 JSON
(--format cyclonedx, the default) or SPDX 2.3 JSON (--format spdx-json): the
authored rules, context, skills, agents, commands, checks, hooks and roles, the
remote includes and skill sources (with their pinned commit), and the MCP servers
(a package URL comes from the server's "package" key, else it is guessed from
npx, uvx, docker run and go run commands; remote servers are listed as services).

The document has no timestamp (SPDX, which requires one, gets a fixed
placeholder) and is byte-identical across runs, operating systems and line
endings. Pass --timestamp (or set SOURCE_DATE_EPOCH) to record a time. ai-rulez
digests appear only in "ai-rulez:" properties, never in the standard hash
fields. Environment and header values are never read, and URLs lose their
credentials and query. The serial number is derived from the lock tree
(ai-rulez.lock) or, without a lock, from the tree computed from the sources.

  --files none|skills|all   list the files of the items with their plain SHA-256
                            (scripts a skill runs are always listed)
  --profile, --role         describe one profile's or role's slice
  --include-outputs         list the generated files with their output digest
  --no-approvals            leave the approval status out
  --redact-reviewers        replace reviewer identities with a salted hash
  --verify                  verify the lock attestation and record the result
  --require-lock            fail (exit 2, AR752) unless ai-rulez.lock matches the sources
  --strict-pins             fail (exit 2, AR750/AR751) when an MCP package or remote
                            source is not pinned to one release or has no package URL
  --check                   with -o: compare the committed SBOM with a fresh one
                            and exit 2 (AR753) when they differ

By default sbom does not touch the network: remote includes and skill sources
come from ai-rulez.lock and the local cache (run "ai-rulez generate" or
"ai-rulez lock" once to fill it). --online lets it contact the remotes, as
generate does, to resolve moving refs.

Sign the document with "ai-rulez sign --sbom". The machine-local overlay
(config.local.*, local/) is never included. Nothing is written unless
--output is given. Exit codes: 0 ok, 1 could not run, 2 a gate failed.`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, _ []string) {
		if code := runSBOM(cmd.OutOrStdout(), cmd.ErrOrStderr(), sbomOpts, cmd.Flags().Changed("timestamp")); code != 0 {
			os.Exit(code)
		}
	},
}

func init() {
	f := SBOMCmd.Flags()
	f.StringVar(&sbomOpts.format, "format", formatCycloneDX, "Output format: cyclonedx (CycloneDX 1.6 JSON) or spdx-json (SPDX 2.3 JSON)")
	f.BoolVar(&sbomOpts.online, "online", false, "Allow contacting remote includes and skill sources (git ls-remote); by default only the lock and the cache are used")
	f.StringVarP(&sbomOpts.output, "output", "o", "", "Write the document to this file instead of stdout")
	f.StringVar(&sbomOpts.files, "files", sbom.FilesNone, "List the files of items with their plain SHA-256: none, skills or all")
	f.StringVar(&sbomOpts.profile, "profile", "", "Describe only this profile's domains")
	f.StringVar(&sbomOpts.role, "role", "", "Describe only the items this role keeps")
	f.BoolVar(&sbomOpts.includeOutputs, "include-outputs", false, "List the generated output files with their output digest")
	f.BoolVar(&sbomOpts.noApprovals, "no-approvals", false, "Leave the approval status of the items out")
	f.BoolVar(&sbomOpts.redactReviewers, "redact-reviewers", false, "Replace reviewer identities with a salted hash")
	f.BoolVar(&sbomOpts.verify, "verify", false, "Verify the lock attestation ([signing] trust) and record the result")
	f.BoolVar(&sbomOpts.requireLock, "require-lock", false, "Fail with exit 2 unless ai-rulez.lock is in sync with the sources")
	f.BoolVar(&sbomOpts.strictPins, "strict-pins", false, "Fail with exit 2 when an MCP package or remote source is not pinned to one release")
	f.BoolVar(&sbomOpts.check, "check", false, "With --output: exit 2 when the committed SBOM differs from a fresh one")
	f.StringVar(&sbomOpts.timestamp, "timestamp", "", "Record this time (RFC 3339, or \"now\"); SOURCE_DATE_EPOCH is honoured when the flag is absent")
	f.Lookup("timestamp").NoOptDefVal = "now"
	f.StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
}

func (f sbomFlags) validate() error {
	if _, err := sbom.NormalizeFormat(f.format); err != nil {
		return err //nolint:wrapcheck // already contextual
	}
	switch f.files {
	case sbom.FilesNone, sbom.FilesSkills, sbom.FilesAll, "":
	default:
		return oops.Hint("use none, skills or all").Errorf("unknown --files %q", f.files)
	}
	if f.check && f.output == "" {
		return oops.Hint("pass -o <committed sbom file>").Errorf("--check compares a committed SBOM, so it needs --output")
	}
	return nil
}

// documentTime resolves --timestamp and SOURCE_DATE_EPOCH.
func documentTime(flag string, flagSet bool, env string, now time.Time) (time.Time, error) {
	switch {
	case flagSet && flag == "now":
		return now.UTC().Truncate(time.Second), nil
	case flagSet:
		t, err := time.Parse(time.RFC3339, flag)
		if err != nil {
			return time.Time{}, oops.Hint(`use an RFC 3339 time such as 2026-01-02T03:04:05Z, or "now"`).Errorf("invalid --timestamp %q", flag)
		}
		return t.UTC(), nil
	case env != "":
		secs, err := strconv.ParseInt(strings.TrimSpace(env), 10, 64)
		if err != nil || secs < 0 {
			return time.Time{}, oops.Errorf("%s %q is not a Unix time in seconds", sourceDateEpochEnv, env)
		}
		return time.Unix(secs, 0).UTC(), nil
	}
	return time.Time{}, nil
}

// sbomRedactKeyEnv names the environment variable that keys --redact-reviewers.
const sbomRedactKeyEnv = "AI_RULEZ_SBOM_REDACT_KEY"

// sbomNow is the wall clock; tests replace it.
var sbomNow = time.Now

// committedTime reads the time a committed SBOM records (CycloneDX
// metadata.timestamp or SPDX creationInfo.created); ok is false when the file
// has none.
func committedTime(path string) (time.Time, bool) {
	data, err := os.ReadFile(path) //nolint:gosec // the path is the --output the user named
	if err != nil {
		return time.Time{}, false
	}
	var doc struct {
		Metadata     struct{ Timestamp string } `json:"metadata"`
		CreationInfo struct{ Created string }   `json:"creationInfo"`
	}
	if json.Unmarshal(data, &doc) != nil {
		return time.Time{}, false
	}
	for _, raw := range []string{doc.Metadata.Timestamp, doc.CreationInfo.Created} {
		if t, err := time.Parse(time.RFC3339, raw); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

// runSBOM builds the document and returns the exit code: 0 ok, 1 it could not
// run, exitDrift when --require-lock, --strict-pins or --check fails.
func runSBOM(out, errOut io.Writer, f sbomFlags, timestampSet bool) int {
	if err := f.validate(); err != nil {
		fmtError(err)
		return 1
	}
	format, _ := sbom.NormalizeFormat(f.format) //nolint:errcheck // validated above
	now := sbomNow()
	if f.check {
		// The approvals are judged at the time the committed document was made, so
		// the check reports changed content, not an approval that expired since.
		if at, ok := committedTime(f.output); ok {
			now = at
		}
	}
	stamp, err := documentTime(f.timestamp, timestampSet, os.Getenv(sourceDateEpochEnv), now)
	if err != nil {
		fmtError(err)
		return 1
	}
	ctx := cmdContext()
	if !f.online {
		ctx = config.WithOfflineIncludes(ctx)
	}
	cfg, err := loadConfigForCommand(config.WithUnresolvedIncludesTolerated(ctx), nil, config.WithoutLocal())
	if err != nil {
		if !f.online {
			err = oops.Hint("sbom reads remote sources from the lock and the cache only; run `ai-rulez generate` or `ai-rulez lock` to fill the cache, or pass --online").Wrap(err)
		}
		fmtError(err)
		return 1
	}
	if err := cfg.Validate(); err != nil {
		fmtError(err)
		return 1
	}
	opts := sbom.Options{
		Files: f.files, IncludeOutputs: f.includeOutputs, Profile: f.profile, Role: f.role,
		NoApprovals: f.noApprovals, RedactReviewers: f.redactReviewers, RedactKey: os.Getenv(sbomRedactKeyEnv), Timestamp: stamp, Now: now,
	}
	if f.verify {
		if opts.Signature, err = lockSignature(cfg); err != nil {
			fmtError(err)
			return 1
		}
	}
	bom, err := sbom.Build(cfg, Version, opts)
	if err != nil {
		fmtError(err)
		return 1
	}
	if code := sbomGates(errOut, f, bom); code != 0 {
		return code
	}
	var buf bytes.Buffer
	if err := sbom.Render(&buf, bom, format); err != nil {
		fmtError(err)
		return 1
	}
	switch {
	case f.check:
		return checkSBOM(out, errOut, f.output, buf.Bytes())
	case f.output == "":
		if _, err := out.Write(buf.Bytes()); err != nil {
			fmtError(oops.Wrapf(err, "write sbom"))
			return 1
		}
		return 0
	}
	if err := os.WriteFile(f.output, buf.Bytes(), sbomFileMode); err != nil { //nolint:gosec // an SBOM is meant to be shared
		fmtError(oops.With("path", f.output).Wrapf(err, "write sbom"))
		return 1
	}
	return 0
}

// sbomGates applies --require-lock and --strict-pins.
func sbomGates(errOut io.Writer, f sbomFlags, bom *sbom.BOM) int {
	var failed []sbom.Finding
	if f.requireLock {
		switch {
		case !bom.LockPresent:
			failed = append(failed, sbom.Finding{Code: sbom.CodeLockStale, Subject: "ai-rulez.lock", Message: "no lock pins this project; run `ai-rulez lock`"})
		case !bom.LockInSync:
			failed = append(failed, sbom.Finding{Code: sbom.CodeLockStale, Subject: "ai-rulez.lock", Message: "the lock does not match the sources; run `ai-rulez lock`, or `ai-rulez lock --diff` to see what changed"})
		}
	}
	if f.strictPins {
		for _, finding := range bom.Findings {
			if finding.Code == sbom.CodeUnpinned || finding.Code == sbom.CodeUnknownCoords {
				failed = append(failed, finding)
			}
		}
	}
	for _, finding := range failed {
		fmt.Fprintln(errOut, finding.String())
	}
	if len(failed) > 0 {
		return exitDrift
	}
	return 0
}

// checkSBOM compares the committed document at path with the fresh one.
func checkSBOM(out, errOut io.Writer, path string, fresh []byte) int {
	committed, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		fmt.Fprintln(errOut, sbom.Finding{Code: sbom.CodeDrift, Subject: path, Message: "no committed SBOM; run `ai-rulez sbom -o " + path + "` and commit it"})
		return exitDrift
	}
	if err != nil {
		fmtError(oops.With("path", path).Wrapf(err, "read the committed sbom"))
		return 1
	}
	diffs, err := sbom.Drift(committed, fresh)
	if err != nil {
		fmtError(err)
		return 1
	}
	if len(diffs) == 0 {
		fmt.Fprintf(out, "%s is up to date\n", path)
		return 0
	}
	fmt.Fprintln(errOut, sbom.Finding{Code: sbom.CodeDrift, Subject: path, Message: "differs from the SBOM generated now; regenerate it with `ai-rulez sbom -o " + path + "`"})
	for _, d := range diffs {
		fmt.Fprintln(errOut, "  "+d)
	}
	return exitDrift
}

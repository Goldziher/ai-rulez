package commands

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/samber/oops"
	"github.com/spf13/cobra"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/jsondoc"
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
	format, docType, output, files, profile, role, timestamp string
	online, includeOutputs, noApprovals                      bool
	redactReviewers, verify                                  bool
	requireLock, strictPins, check                           bool
}

var sbomOpts sbomFlags

// SBOMCmd prints the project's AI configuration as a CycloneDX or SPDX bill of materials.
var SBOMCmd = &cobra.Command{
	Use:   "sbom",
	Short: "Print a CycloneDX or SPDX software bill of materials of the AI configuration",
	Long: `Print a bill of materials of the project's AI configuration, as CycloneDX 1.6 JSON
(--type cyclonedx, the default) or SPDX 2.3 JSON (--type spdx-json): the
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
--output is given.

The bill of materials is JSON whatever --format says. --format json changes only
the report of a run that has no document to print: a failed gate, --check, or a
document written with --output; it then prints {schema_version, status, type,
output, findings, differences} (schema/sbom-report.schema.json) on stdout.
Exit codes: 0 ok, 1 could not run, 2 a gate failed.`,
	Args: sbomArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runSBOM(cmd.OutOrStdout(), cmd.ErrOrStderr(), sbomOpts, cmd.Flags().Changed("timestamp"))
	},
}

func init() {
	f := SBOMCmd.Flags()
	f.StringVar(&sbomOpts.docType, "type", formatCycloneDX, "Document type: cyclonedx (CycloneDX 1.6 JSON) or spdx-json (SPDX 2.3 JSON)")
	addFormatFlag(f, &sbomOpts.format, "", formatText, formatText, formatJSON)
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
	f.StringVar(&sbomOpts.timestamp, "timestamp", "", "Record this time (RFC 3339, or \"now\"); SOURCE_DATE_EPOCH is honored when the flag is absent")
	f.Lookup("timestamp").NoOptDefVal = "now"
	f.StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
}

func (f sbomFlags) validate() error {
	if _, err := sbom.NormalizeFormat(f.docType); err != nil {
		return err //nolint:wrapcheck // already contextual
	}
	if err := checkFormatFlag(f.format); err != nil {
		return err
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

// sbomArgs rejects positional arguments. --timestamp takes an optional value, so
// "--timestamp bogus" leaves "bogus" as an argument; that is a bad value, not an
// unknown subcommand.
func sbomArgs(cmd *cobra.Command, args []string) error {
	if len(args) > 0 && cmd.Flags().Changed("timestamp") {
		return oops.Hint(`write the value as --timestamp=VALUE (an RFC 3339 time, or "now")`).
			Errorf("invalid --timestamp: %q is not a flag value", args[0])
	}
	return cobra.NoArgs(cmd, args)
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

// loadSBOMConfig loads and validates the configuration; offline unless online,
// so remote sources come from the lock and the cache.
func loadSBOMConfig(online bool) (*config.Config, error) {
	ctx := cmdContext()
	if !online {
		ctx = config.WithOfflineIncludes(ctx)
	}
	cfg, err := loadConfigForCommand(config.WithUnresolvedIncludesTolerated(ctx), nil, config.WithoutLocal())
	if err != nil {
		if !online {
			err = oops.Hint("sbom reads remote sources from the lock and the cache only; run `ai-rulez generate` or `ai-rulez lock` to fill the cache, or pass --online").Wrap(err)
		}
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// sbomReport is the `sbom --format json` document of a run that reports instead
// of printing the bill of materials: a gate that failed, --check, or a document
// written with --output. Without --output the document on stdout is the BOM
// itself, which is JSON in both formats.
type sbomReport struct {
	SchemaVersion int    `json:"schema_version"`
	Status        string `json:"status"`
	Type          string `json:"type"`
	Output        string `json:"output,omitempty"`
	// Findings are the gate failures (--require-lock, --strict-pins, --check).
	Findings []sbomReportFinding `json:"findings"`
	// Differences are the lines that differ from the committed SBOM (--check).
	Differences []string `json:"differences"`
}

type sbomReportFinding struct {
	Code    string `json:"code"`
	Name    string `json:"name"`
	Subject string `json:"subject"`
	Message string `json:"message"`
}

const (
	sbomStatusOK       = "ok"
	sbomStatusFindings = "findings"
	sbomStatusDrift    = "drift"
)

// sbomRun carries what the steps of one run share.
type sbomRun struct {
	out, errOut io.Writer
	json        bool
	docType     string
	output      string
}

func (r sbomRun) report(status string, findings []sbom.Finding, diffs []string) error {
	doc := sbomReport{SchemaVersion: 1, Status: status, Type: r.docType, Output: r.output, Findings: []sbomReportFinding{}, Differences: []string{}}
	for _, f := range findings {
		doc.Findings = append(doc.Findings, sbomReportFinding{Code: f.Code, Name: sbom.CodeName(f.Code), Subject: f.Subject, Message: f.Message})
	}
	doc.Differences = append(doc.Differences, diffs...)
	if err := jsondoc.Write(r.out, doc); err != nil {
		return fail(err)
	}
	return nil
}

// runSBOM builds the document. It returns nil on success, an ExitError with
// exitFindings when --require-lock, --strict-pins or --check fails, and the
// failure otherwise.
func runSBOM(out, errOut io.Writer, f sbomFlags, timestampSet bool) error {
	if err := f.validate(); err != nil {
		return fail(err)
	}
	docType, _ := sbom.NormalizeFormat(f.docType) //nolint:errcheck // validated above
	run := sbomRun{out: out, errOut: errOut, json: f.format == formatJSON, docType: docType, output: f.output}
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
		return fail(err)
	}
	cfg, err := loadSBOMConfig(f.online)
	if err != nil {
		return fail(err)
	}
	opts := sbom.Options{
		Files: f.files, IncludeOutputs: f.includeOutputs, Profile: f.profile, Role: f.role,
		NoApprovals: f.noApprovals, RedactReviewers: f.redactReviewers, RedactKey: os.Getenv(sbomRedactKeyEnv), Timestamp: stamp, Now: now,
	}
	if f.verify {
		if opts.Signature, err = lockSignature(cfg); err != nil {
			return fail(err)
		}
	}
	bom, err := sbom.Build(cfg, Version, opts)
	if err != nil {
		return fail(err)
	}
	if err := run.gates(f, bom); err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := sbom.Render(&buf, bom, docType); err != nil {
		return fail(err)
	}
	switch {
	case f.check:
		return run.check(f.output, buf.Bytes())
	case f.output == "":
		if _, err := out.Write(buf.Bytes()); err != nil {
			return failMsg("write sbom", err)
		}
		return nil
	}
	if err := os.WriteFile(f.output, buf.Bytes(), sbomFileMode); err != nil { //nolint:gosec // an SBOM is meant to be shared
		return fail(oops.With("path", f.output).Wrapf(err, "write sbom"))
	}
	if run.json {
		return run.report(sbomStatusOK, nil, nil)
	}
	return nil
}

// gates applies --require-lock and --strict-pins.
func (r sbomRun) gates(f sbomFlags, bom *sbom.BOM) error {
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
	if len(failed) == 0 {
		return nil
	}
	if r.json {
		if err := r.report(sbomStatusFindings, failed, nil); err != nil {
			return err
		}
		return exitStatus(exitFindings)
	}
	for _, finding := range failed {
		reportWriter{r.errOut}.printf("%s\n", finding.String())
	}
	return exitStatus(exitFindings)
}

// check compares the committed document at path with the fresh one.
func (r sbomRun) check(path string, fresh []byte) error {
	committed, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		finding := sbom.Finding{Code: sbom.CodeDrift, Subject: path, Message: "no committed SBOM; run `ai-rulez sbom -o " + path + "` and commit it"}
		return r.drift(finding, nil)
	}
	if err != nil {
		return fail(oops.With("path", path).Wrapf(err, "read the committed sbom"))
	}
	diffs, err := sbom.Drift(committed, fresh)
	if err != nil {
		return fail(err)
	}
	if len(diffs) == 0 {
		if r.json {
			return r.report(sbomStatusOK, nil, nil)
		}
		reportWriter{r.out}.printf("%s is up to date\n", path)
		return nil
	}
	finding := sbom.Finding{Code: sbom.CodeDrift, Subject: path, Message: "differs from the SBOM generated now; regenerate it with `ai-rulez sbom -o " + path + "`"}
	return r.drift(finding, diffs)
}

func (r sbomRun) drift(finding sbom.Finding, diffs []string) error {
	if r.json {
		if err := r.report(sbomStatusDrift, []sbom.Finding{finding}, diffs); err != nil {
			return err
		}
		return exitStatus(exitFindings)
	}
	reportWriter{r.errOut}.printf("%s\n", finding.String())
	for _, d := range diffs {
		reportWriter{r.errOut}.printf("%s\n", "  "+d)
	}
	return exitStatus(exitFindings)
}

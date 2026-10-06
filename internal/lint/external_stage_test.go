package lint

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

const stagedSkill = "---\nname: deploy\ndescription: Use when deploying the service to production.\n---\n# Deploy\n\nRun the script.\n"

// scannerProject is a fixture project with one fake scanner script. run lints
// it again each call, so a test can change files, options or the baseline between runs.
type scannerProject struct {
	t    *testing.T
	root string
}

func newScannerProject(t *testing.T, scriptBody, keys string, extra map[string]string) *scannerProject {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake scanners are POSIX shell scripts")
	}
	root := t.TempDir()
	script := filepath.Join(t.TempDir(), "scan.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n"+scriptBody), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		".ai-rulez/rules/r.md":                       "# Rule\n\nline two\nBADTOKEN here\n",
		".ai-rulez/skills/deploy/SKILL.md":           stagedSkill,
		".ai-rulez/skills/deploy/scripts/run.sh":     "#!/bin/sh\necho deploy\n",
		".ai-rulez/skills/deploy/references/note.md": "# Note\n",
		".env":    "SECRET_TOKEN=abc\n",
		"main.go": "package main\n",
		".ai-rulez/config.toml": baseConfig + "\n[[lint.external]]\nname = \"fake\"\ncommand = [\"" +
			filepath.ToSlash(script) + "\"" + commandTail(keys) + "]\n" + stripCommandTail(keys),
	}
	for k, v := range extra {
		if k == "config.pre" { // TOML placed before the scanner table
			files[".ai-rulez/config.toml"] = baseConfig + v + strings.TrimPrefix(files[".ai-rulez/config.toml"], baseConfig)
			continue
		}
		files[k] = v
	}
	writeFiles(t, root, files)
	gitAdd(t, root)
	return &scannerProject{t: t, root: root}
}

// commandTail lets a test append arguments to the command: a line "args = ..." in keys.
func commandTail(keys string) string {
	for _, line := range strings.Split(keys, "\n") {
		if rest, ok := strings.CutPrefix(line, "args = "); ok {
			return ", " + rest
		}
	}
	return ""
}

func stripCommandTail(keys string) string {
	var out []string
	for _, line := range strings.Split(keys, "\n") {
		if !strings.HasPrefix(line, "args = ") {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

func (p *scannerProject) run(opts Options) []Finding {
	p.t.Helper()
	cfg := loadNoRemote(p.t, p.root)
	tree, err := LoadTree(p.root)
	if err != nil {
		p.t.Fatal(err)
	}
	opts.External = true
	rep, err := RunWith(cfg, tree, opts)
	if err != nil {
		p.t.Fatal(err)
	}
	return rep.Findings
}

func (p *scannerProject) write(rel, body string) {
	p.t.Helper()
	writeFiles(p.t, p.root, map[string]string{rel: body})
}

func ofCode(fs []Finding, code string) []Finding {
	var out []Finding
	for _, f := range fs {
		if f.Code == code {
			out = append(out, f)
		}
	}
	return out
}

// sarifMessage is a script line that prints a one-result SARIF whose message is $MSG.
const sarifMessage = `printf '{"version":"2.1.0","runs":[{"results":[{"ruleId":"SEE","level":"note","message":{"text":"%s"}}]}]}' "$MSG"` + "\n"

func TestStagedInputsAreTheOnlyFilesTheScannerSees(t *testing.T) {
	// Arrange: a scanner that lists what it can see and where it runs.
	script := `MSG="cwd=$(pwd | sed 's:.*/::') files=$(find . -type f | sort | tr '\n' ' ') home=$(echo "$HOME" | sed 's:.*/::') ` +
		`writable=$([ -w . ] && echo yes || echo no) filewritable=$([ -w .ai-rulez/skills/deploy/SKILL.md ] && echo yes || echo no)"` + "\n" + sarifMessage
	tests := []struct {
		name     string
		inputs   string
		want     []string
		notWant  []string
		wantSkip bool
	}{
		{
			name: "skills only", inputs: `["skills"]`,
			want: []string{
				"cwd=stage", "./.ai-rulez/skills/deploy/SKILL.md", "./.ai-rulez/skills/deploy/scripts/run.sh",
				"./.ai-rulez/skills/deploy/references/note.md", "home=home", "writable=no", "filewritable=no",
			},
			notWant: []string{".env", "main.go", "config.toml", "rules/r.md"},
		},
		{
			name: "rules and skills", inputs: `["rules", "skills"]`,
			want:    []string{"./.ai-rulez/rules/r.md", "./.ai-rulez/skills/deploy/SKILL.md"},
			notWant: []string{".env", "main.go", "config.toml"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newScannerProject(t, script, "egress = false\ninputs = "+tt.inputs+"\n", nil)
			// Act
			findings := p.run(Options{})
			// Assert
			got := ofCode(findings, CodeExternalFinding)
			if len(got) != 1 {
				t.Fatalf("want one AR011:\n%s", dump(findings))
			}
			for _, w := range tt.want {
				if !strings.Contains(got[0].Message, w) {
					t.Errorf("message lacks %q: %s", w, got[0].Message)
				}
			}
			for _, w := range tt.notWant {
				if strings.Contains(got[0].Message, w) {
					t.Errorf("message must not contain %q: %s", w, got[0].Message)
				}
			}
		})
	}
}

func TestStagedPlaceholdersExpandAsArguments(t *testing.T) {
	// Arrange: the scanner takes {skill_dirs} and {out}; it writes SARIF to the out file and prints nothing.
	script := `OUT="$2"
DIR="$1"
printf '{"version":"2.1.0","runs":[{"results":[{"ruleId":"R","level":"error","message":{"text":"dir=%s"},"locations":[{"physicalLocation":{"artifactLocation":{"uri":"SKILL.md"},"region":{"startLine":2}}}]}]}]}' "$(basename "$DIR")" > "$OUT"
`
	// {skill_dirs} is $1, --out is $2 and {out} is $3.
	p := newScannerProject(t, strings.Replace(script, `OUT="$2"`, `OUT="$3"`, 1),
		"egress = false\ninputs = [\"skills\"]\nargs = \"{skill_dirs}\", \"--out\", \"{out}\"\n", nil)
	// Act
	findings := p.run(Options{})
	// Assert: the relative path "SKILL.md" is found through the staged skill directory.
	got := ofCode(findings, CodeExternalFinding)
	if len(got) != 1 {
		t.Fatalf("want one AR011:\n%s", dump(findings))
	}
	if !strings.Contains(got[0].Message, "dir=deploy") || got[0].Severity != SeverityError {
		t.Errorf("got %+v", got[0])
	}
	if !strings.HasSuffix(got[0].File, ".ai-rulez/skills/deploy/SKILL.md") || got[0].Line != 2 {
		t.Errorf("finding is attributed to %s:%d, want the project's SKILL.md:2", got[0].File, got[0].Line)
	}
	if len(ofCode(findings, CodeScannerOutOfScope)) != 0 {
		t.Errorf("unexpected AR9E6:\n%s", dump(findings))
	}
}

func TestStagedOutOfScopeResultsAreDropped(t *testing.T) {
	// Arrange: a scanner given only skills reports on a rule file, an absolute path, an escape and a staged file.
	script := `printf '{"version":"2.1.0","runs":[{"results":[` +
		`{"ruleId":"A","message":{"text":"rule file"},"locations":[{"physicalLocation":{"artifactLocation":{"uri":".ai-rulez/rules/r.md"}}}]},` +
		`{"ruleId":"B","message":{"text":"abs"},"locations":[{"physicalLocation":{"artifactLocation":{"uri":"/etc/passwd"}}}]},` +
		`{"ruleId":"C","message":{"text":"escape"},"locations":[{"physicalLocation":{"artifactLocation":{"uri":"../../main.go"}}}]},` +
		`{"ruleId":"D","message":{"text":"ok"},"locations":[{"physicalLocation":{"artifactLocation":{"uri":".ai-rulez/skills/deploy/SKILL.md"},"region":{"startLine":1}}}]}]}]}'` + "\n"
	p := newScannerProject(t, script, "egress = false\ninputs = [\"skills\"]\n", nil)
	// Act
	findings := p.run(Options{})
	// Assert
	got := ofCode(findings, CodeExternalFinding)
	if len(got) != 1 || !strings.Contains(got[0].Message, "ok") {
		t.Fatalf("want only the staged result:\n%s", dump(findings))
	}
	oos := ofCode(findings, CodeScannerOutOfScope)
	if len(oos) != 1 || oos[0].Severity != SeverityWarning || !strings.Contains(oos[0].Message, "3 result(s)") {
		t.Fatalf("want one AR9E6 counting 3:\n%s", dump(findings))
	}
}

func TestStagedWithNothingToScanDoesNotRun(t *testing.T) {
	// Arrange: inputs name a kind the project has none of; the scanner would fail if it ran.
	p := newScannerProject(t, "exit 9\n", "egress = false\ninputs = [\"agents\"]\n", nil)
	// Act
	findings := p.run(Options{})
	// Assert
	for _, code := range []string{CodeScannerRunFailed, CodeExternalFinding, CodeScannerUnavailable} {
		if got := ofCode(findings, code); len(got) != 0 {
			t.Errorf("unexpected %s:\n%s", code, dump(findings))
		}
	}
}

func TestStagedEnvironmentIsScrubbed(t *testing.T) {
	t.Setenv("AR_PLANTED_API_KEY", "planted-value")
	t.Setenv("AR_KEEP_ME", "kept")
	script := `MSG="planted=$(env | grep -c AR_PLANTED) kept=$(env | grep -c AR_KEEP) tmp=$(echo "$TMPDIR" | sed 's:.*/::')"` + "\n" + sarifMessage
	p := newScannerProject(t, script, "egress = false\ninputs = [\"skills\"]\nenv_pass = [\"AR_KEEP_ME\"]\n", nil)
	// Act
	got := ofCode(p.run(Options{}), CodeExternalFinding)
	// Assert
	if len(got) != 1 || !strings.Contains(got[0].Message, "planted=0 kept=1 tmp=tmp") {
		t.Fatalf("got %+v", got)
	}
}

func TestStagedScratchIsRemoved(t *testing.T) {
	// Arrange: the scanner prints the scratch directory it ran in.
	script := `MSG="$(dirname "$(pwd)")"` + "\n" + sarifMessage
	p := newScannerProject(t, script, "egress = false\ninputs = [\"skills\"]\n", nil)
	// Act
	got := ofCode(p.run(Options{}), CodeExternalFinding)
	// Assert
	if len(got) != 1 {
		t.Fatalf("want one finding, got %+v", got)
	}
	scratch := strings.TrimSpace(strings.SplitN(strings.TrimPrefix(got[0].Message, "[fake] SEE: "), " ", 2)[0])
	if _, err := os.Stat(scratch); !os.IsNotExist(err) {
		t.Errorf("scratch directory %s still exists (err %v)", scratch, err)
	}
}

func TestStagedExcludesSymlinkedContent(t *testing.T) {
	// Arrange: a skill resource that is a symlink to a file outside the project.
	secret := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(secret, []byte("TOPSECRET\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	script := `MSG="$(grep -rl TOPSECRET . | wc -l | tr -d ' ') hits"` + "\n" + sarifMessage
	p := newScannerProject(t, script, "egress = false\ninputs = [\"skills\"]\n", nil)
	link := filepath.Join(p.root, ".ai-rulez/skills/deploy/references/link.txt")
	testutil.SymlinkOrSkip(t, secret, link)
	// Act
	got := ofCode(p.run(Options{}), CodeExternalFinding)
	// Assert
	if len(got) != 1 || !strings.Contains(got[0].Message, "0 hits") {
		t.Fatalf("a symlinked file must never reach the scanner: %+v", got)
	}
}

func TestStagedConfigProblems(t *testing.T) {
	tests := []struct {
		name string
		keys string
		want string
	}{
		{"unknown input", "egress = false\ninputs = [\"everything\"]\n", `inputs "everything"`},
		{"placeholder without inputs", "egress = false\nargs = \"{stage}\"\n", "needs inputs"},
		{"unknown placeholder", "egress = false\ninputs = [\"skills\"]\nargs = \"{nope}\"\n", "unknown placeholder {nope}"},
		{"list placeholder inside a word", "egress = false\ninputs = [\"skills\"]\nargs = \"--in={files}\"\n", "argument on its own"},
		{"bad severity_map value", "egress = false\nseverity_map = { \"X*\" = \"scary\" }\n", "severity_map"},
		{"bad max_severity", "egress = false\nmax_severity = \"huge\"\n", "max_severity"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newScannerProject(t, "exit 9\n", tt.keys, nil)
			// Act
			findings := p.run(Options{})
			// Assert
			got := ofCode(findings, CodeScannerConfigInvalid)
			if len(got) == 0 || !strings.Contains(got[0].Message, tt.want) {
				t.Fatalf("want AR9E0 containing %q:\n%s", tt.want, dump(findings))
			}
			if len(ofCode(findings, CodeScannerRunFailed)) != 0 {
				t.Errorf("a scanner with an invalid entry must not run:\n%s", dump(findings))
			}
		})
	}
}

// lineScanner reports BADTOKEN at the line it is on, with an optional SARIF partial fingerprint in $FP.
const lineScanner = `N=$(grep -n BADTOKEN .ai-rulez/rules/r.md | head -1 | cut -d: -f1)
FPJSON=""
[ -n "$FP" ] && FPJSON=',"partialFingerprints":{"primaryLocationLineHash":"'"$FP"'"}'
printf '{"version":"2.1.0","runs":[{"results":[{"ruleId":"TOK","level":"warning","message":{"text":"bad token"},"locations":[{"physicalLocation":{"artifactLocation":{"uri":".ai-rulez/rules/r.md"},"region":{"startLine":%s}}}]%s}]}]}' "$N" "$FPJSON"
`

func scannerFingerprintOf(t *testing.T, findings []Finding) string {
	t.Helper()
	got := ofCode(findings, CodeExternalFinding)
	if len(got) != 1 {
		t.Fatalf("want one AR011:\n%s", dump(findings))
	}
	fp := got[0].Fingerprint()
	if !strings.HasPrefix(fp, scannerFingerprintVersion+":") {
		t.Fatalf("fingerprint %q is not a scanner fingerprint", fp)
	}
	return fp
}

func TestScannerFingerprintStability(t *testing.T) {
	const rule = ".ai-rulez/rules/r.md"
	tests := []struct {
		name   string
		mutate func(p *scannerProject)
		same   bool
	}{
		{"unchanged", func(*scannerProject) {}, true},
		{"lines inserted above", func(p *scannerProject) { p.write(rule, "# Rule\n\nnew\nlines\nabove\nline two\nBADTOKEN here\n") }, true},
		{"flagged line reindented", func(p *scannerProject) { p.write(rule, "# Rule\n\nline two\n    BADTOKEN   here  \n") }, true},
		{"flagged line edited", func(p *scannerProject) { p.write(rule, "# Rule\n\nline two\nBADTOKEN there\n") }, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newScannerProject(t, lineScanner, "egress = false\ninputs = [\"rules\"]\n", nil)
			before := scannerFingerprintOf(t, p.run(Options{}))
			// Act
			tt.mutate(p)
			after := scannerFingerprintOf(t, p.run(Options{}))
			// Assert
			if (before == after) != tt.same {
				t.Errorf("fingerprint before %s after %s, same = %v want %v", before, after, before == after, tt.same)
			}
		})
	}
}

func TestScannerFingerprintUsesTheScannersOwn(t *testing.T) {
	// Arrange: the scanner gives a partial fingerprint; editing the flagged line must not change it.
	t.Setenv("FP", "abc123:1")
	p := newScannerProject(t, lineScanner, "egress = false\ninputs = [\"rules\"]\nenv_pass = [\"FP\"]\n", nil)
	before := scannerFingerprintOf(t, p.run(Options{}))
	// Act
	p.write(".ai-rulez/rules/r.md", "# Rule\n\nline two\nBADTOKEN edited completely\n")
	after := scannerFingerprintOf(t, p.run(Options{}))
	t.Setenv("FP", "other")
	other := scannerFingerprintOf(t, p.run(Options{}))
	// Assert
	if before != after {
		t.Errorf("own fingerprint must survive an edit of the line: %s vs %s", before, after)
	}
	if before == other {
		t.Errorf("a different scanner fingerprint must give a different identity")
	}
}

func TestScannerBaselineLifecycle(t *testing.T) {
	// Arrange
	p := newScannerProject(t, lineScanner, "egress = false\ninputs = [\"rules\"]\n", nil)
	baseline := filepath.Join(p.root, ".ai-rulez", ScannerBaselineFile)
	if got := ofCode(p.run(Options{}), CodeExternalFinding); len(got) != 1 {
		t.Fatalf("want a finding before the baseline, got %d", len(got))
	}

	// Act and assert: writing without a reason is refused and writes nothing.
	findings := p.run(Options{Scanner: ScannerOptions{WriteBaseline: true}})
	if got := ofCode(findings, CodeScannerConfigInvalid); len(got) != 1 || !strings.Contains(got[0].Message, "reason") {
		t.Fatalf("want an AR9E0 asking for a reason:\n%s", dump(findings))
	}
	if _, err := os.Stat(baseline); err == nil {
		t.Fatal("a refused write must not create the baseline")
	}

	// Writing with a reason accepts the finding and the run is clean.
	findings = p.run(Options{Scanner: ScannerOptions{WriteBaseline: true, Reason: "test fixture"}})
	if got := ofCode(findings, CodeExternalFinding); len(got) != 0 {
		t.Fatalf("a written baseline accepts its findings:\n%s", dump(findings))
	}
	b, err := LoadBaseline(baseline)
	if err != nil || b == nil || len(b.Entries) != 1 {
		t.Fatalf("baseline = %+v, %v", b, err)
	}
	e := b.Entries[0]
	if e.Scanner != "fake" || e.Rule != "TOK" || e.Reason != "test fixture" || e.File != ".ai-rulez/rules/r.md" {
		t.Errorf("entry = %+v", e)
	}

	// A later run drops the accepted finding.
	if got := ofCode(p.run(Options{}), CodeExternalFinding); len(got) != 0 {
		t.Fatalf("an accepted finding must not be reported:\n%s", dump(got))
	}

	// Editing the flagged line makes the finding new again.
	p.write(".ai-rulez/rules/r.md", "# Rule\n\nline two\nBADTOKEN changed\n")
	if got := ofCode(p.run(Options{}), CodeExternalFinding); len(got) != 1 {
		t.Fatalf("an edited line must be reviewed again, got %d findings", len(got))
	}
	p.write(".ai-rulez/rules/r.md", "# Rule\n\nline two\nBADTOKEN here\n")

	// An expired entry resurfaces the finding and adds AR9E5.
	b.Entries[0].Expires = "2026-01-31"
	if err := b.Save(baseline); err != nil {
		t.Fatal(err)
	}
	findings = p.run(Options{Scanner: ScannerOptions{Today: "2026-02-01"}})
	if len(ofCode(findings, CodeExternalFinding)) != 1 {
		t.Errorf("an expired entry must report the finding:\n%s", dump(findings))
	}
	expired := ofCode(findings, CodeScannerBaselineExpired)
	if len(expired) != 1 || expired[0].Severity != SeverityWarning || !strings.Contains(expired[0].Message, "2026-01-31") {
		t.Errorf("want one AR9E5:\n%s", dump(findings))
	}
	// On the expiry day the entry still applies.
	findings = p.run(Options{Scanner: ScannerOptions{Today: "2026-01-31"}})
	if len(ofCode(findings, CodeExternalFinding)) != 0 || len(ofCode(findings, CodeScannerBaselineExpired)) != 0 {
		t.Errorf("the expiry day still accepts:\n%s", dump(findings))
	}
}

func TestScannerBaselineKeepsEntriesOfScannersThatDidNotRun(t *testing.T) {
	// Arrange: a baseline with an entry of a scanner that is not configured now.
	old := &Baseline{Version: baselineVersion, Entries: []BaselineEntry{{
		Fingerprint: "sc1:ghost", Code: CodeExternalFinding, File: "x.md", Scanner: "gone", Rule: "R", Reason: "kept",
	}}}
	p := newScannerProject(t, lineScanner, "egress = false\ninputs = [\"rules\"]\n", nil)
	baseline := filepath.Join(p.root, ".ai-rulez", ScannerBaselineFile)
	if err := old.Save(baseline); err != nil {
		t.Fatal(err)
	}
	// Act
	p.run(Options{Scanner: ScannerOptions{WriteBaseline: true, Reason: "r"}})
	// Assert
	b, err := LoadBaseline(baseline)
	if err != nil || b == nil {
		t.Fatalf("load: %v", err)
	}
	var scanners []string
	for _, e := range b.Entries {
		scanners = append(scanners, e.Scanner)
	}
	if strings.Join(scanners, ",") != "fake,gone" && strings.Join(scanners, ",") != "gone,fake" {
		t.Errorf("entries = %v, want fake and gone", scanners)
	}
}

func TestScannerSeverityMappingEndToEnd(t *testing.T) {
	// Arrange: three results: a mapped rule id, a security-severity score and a rule default level.
	script := `printf '{"version":"2.1.0","runs":[{"tool":{"driver":{"rules":[` +
		`{"id":"DEF","defaultConfiguration":{"level":"error"},"helpUri":"https://example.test/DEF"},` +
		`{"id":"SCORE","properties":{"security-severity":"9.8"}}]}},"results":[` +
		`{"ruleId":"YARA-1","level":"error","message":{"text":"mapped"}},` +
		`{"ruleId":"SCORE","ruleIndex":1,"message":{"text":"scored"}},` +
		`{"ruleId":"DEF","message":{"text":"default"}}]}]}'` + "\n"
	tests := []struct {
		name string
		keys string
		want map[string]Severity
	}{
		{
			name: "defaults", keys: "egress = false\n",
			want: map[string]Severity{"mapped": SeverityError, "scored": SeverityError, "default": SeverityError},
		},
		{
			name: "severity_map first, max_severity caps",
			keys: "egress = false\nseverity_map = { \"YARA-*\" = \"low\" }\nmax_severity = \"warning\"\n",
			want: map[string]Severity{"mapped": SeverityInfo, "scored": SeverityWarning, "default": SeverityWarning},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newScannerProject(t, script, tt.keys, nil)
			// Act
			got := ofCode(p.run(Options{}), CodeExternalFinding)
			// Assert
			if len(got) != 3 {
				t.Fatalf("want 3 findings, got %s", dump(got))
			}
			for _, f := range got {
				for word, sev := range tt.want {
					if strings.Contains(f.Message, word+" ") || strings.HasSuffix(f.Message, word) || strings.Contains(f.Message, word+" (") {
						if f.Severity != sev {
							t.Errorf("%s: severity %s, want %s", word, f.Severity, sev)
						}
					}
				}
			}
			if !strings.Contains(dump(got), "https://example.test/DEF") {
				t.Errorf("the rule's helpUri should be shown as evidence:\n%s", dump(got))
			}
		})
	}
}

func TestShowSuppressedKeepsSuppressedResultsAsInfo(t *testing.T) {
	script := `printf '{"version":"2.1.0","runs":[{"results":[{"ruleId":"S","level":"error","message":{"text":"hidden"},"suppressions":[{"kind":"inSource"}]}]}]}'` + "\n"
	p := newScannerProject(t, script, "egress = false\n", nil)
	// Act and assert
	if got := ofCode(p.run(Options{}), CodeExternalFinding); len(got) != 0 {
		t.Fatalf("suppressed results are dropped by default: %s", dump(got))
	}
	got := ofCode(p.run(Options{Scanner: ScannerOptions{ShowSuppressed: true}}), CodeExternalFinding)
	if len(got) != 1 || got[0].Severity != SeverityInfo || !strings.Contains(got[0].Message, "(suppressed)") {
		t.Fatalf("want one info finding, got %s", dump(got))
	}
}

func TestStagedHooksAndMCPOmitSecrets(t *testing.T) {
	// Arrange
	pre := "\n[[hooks]]\nevent = \"PreToolUse\"\nmatcher = \"Bash\"\n[[hooks.hooks]]\ncommand = \"echo guarded\"\n" +
		"\n[[mcp_servers]]\nname = \"srv\"\ncommand = \"npx\"\nargs = [\"-y\", \"srv\"]\n[mcp_servers.env]\nAPI_TOKEN = \"hunter2\"\n"
	script := `MSG="$(cat .ai-rulez/hooks.json .ai-rulez/mcp-servers.json | tr -d ' \n\"')"` + "\n" + sarifMessage
	p := newScannerProject(t, script, "egress = false\ninputs = [\"hooks\", \"mcp\"]\n", map[string]string{"config.pre": pre})
	// Act
	got := ofCode(p.run(Options{}), CodeExternalFinding)
	// Assert
	if len(got) != 1 {
		t.Fatalf("want one finding, got %+v", got)
	}
	for _, want := range []string{"PreToolUse", "echoguarded", "name:srv", "env_names:[API_TOKEN]"} {
		if !strings.Contains(got[0].Message, want) {
			t.Errorf("message lacks %q: %s", want, got[0].Message)
		}
	}
	if strings.Contains(got[0].Message, "hunter2") {
		t.Errorf("an environment value must never be staged: %s", got[0].Message)
	}
}

func TestSecurityOnlyKeepsScannerRunFindings(t *testing.T) {
	// Arrange: `scan --external` is the security-only run; a broken scanner must not vanish from it.
	p := newScannerProject(t, "echo not sarif\n", "egress = false\n", nil)
	// Act
	findings := p.run(Options{SecurityOnly: true})
	// Assert
	if got := ofCode(findings, CodeScannerRunFailed); len(got) != 1 {
		t.Fatalf("a failing scanner must be reported by the security-only run:\n%s", dump(findings))
	}
}

func TestStagedFileKeepsItsFrontmatterSoLineNumbersMatch(t *testing.T) {
	// Arrange: the scanner reports the line of "Run the script" as it sees it.
	script := `N=$(grep -n "Run the script" .ai-rulez/skills/deploy/SKILL.md | cut -d: -f1)
printf '{"version":"2.1.0","runs":[{"results":[{"ruleId":"L","message":{"text":"m"},"locations":[{"physicalLocation":{"artifactLocation":{"uri":".ai-rulez/skills/deploy/SKILL.md"},"region":{"startLine":%s}}}]}]}]}' "$N"
`
	p := newScannerProject(t, script, "egress = false\ninputs = [\"skills\"]\n", nil)
	// Act
	got := ofCode(p.run(Options{}), CodeExternalFinding)
	// Assert
	if len(got) != 1 || got[0].Line != 7 {
		t.Fatalf("want the line of the real file (7), got %+v", got)
	}
}

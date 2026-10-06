package lint

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/sandbox"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// policyProject builds a project from raw config TOML (after baseConfig) and
// the staged-scanner fixture files.
func policyProject(t *testing.T, tomlBody string) *scannerProject {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake scanners are POSIX shell scripts")
	}
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		".ai-rulez/rules/r.md":                       "# Rule\n\nline two\nBADTOKEN here\n",
		".ai-rulez/skills/deploy/SKILL.md":           stagedSkill,
		".ai-rulez/skills/deploy/scripts/run.sh":     "#!/bin/sh\necho deploy\n",
		".ai-rulez/skills/deploy/references/note.md": "# Note\n",
		".ai-rulez/config.toml":                      baseConfig + tomlBody,
	})
	gitAdd(t, root)
	return &scannerProject{t: t, root: root}
}

// fakeBin installs an executable shell script called name on PATH for the test.
func fakeBin(t *testing.T, name, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755)) //nolint:gosec // a test script
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return path
}

func sarifFor(rule, level, msg string) string {
	return fmt.Sprintf(`{"version":"2.1.0","runs":[{"results":[{"ruleId":%q,"level":%q,"message":{"text":%q}}]}]}`, rule, level, msg)
}

func scannerFindings(fs []Finding) []Finding { return ofCode(fs, CodeExternalFinding) }

func TestPresetRunsItsMembersAndNotesTheMissingOnes(t *testing.T) {
	// Arrange: only agnix is installed; the strict preset also names the Cisco scanner.
	fakeBin(t, "agnix", "echo '"+sarifFor("AGX1", "warning", "structure problem")+"'\n")
	p := policyProject(t, "\n[lint.scanner_policy]\npreset = \"strict\"\n")
	// Act
	findings := p.run(Options{})
	// Assert
	got := scannerFindings(findings)
	require.Len(t, got, 1, dump(findings))
	assert.Contains(t, got[0].Message, "[agnix]")
	assert.Equal(t, SeverityWarning, got[0].Severity)
	missing := ofCode(findings, CodeScannerUnavailable)
	require.Len(t, missing, 1, dump(findings))
	assert.Contains(t, missing[0].Message, "[cisco-skill-scanner]")
	assert.Equal(t, SeverityWarning, missing[0].Severity)
	// The strict preset implies fail_on = warning for scanner findings.
	assert.True(t, Failed(findings, "error"), "a scanner warning must fail under the strict preset")
}

func TestPresetWithoutExternalRunsNothing(t *testing.T) {
	fakeBin(t, "agnix", "echo '"+sarifFor("AGX1", "error", "x")+"'\n")
	p := policyProject(t, "\n[lint.scanner_policy]\npreset = \"baseline\"\n")
	cfg := loadNoRemote(t, p.root)
	tree, err := LoadTree(p.root)
	require.NoError(t, err)
	rep, err := RunWith(cfg, tree, Options{}) // no --external
	require.NoError(t, err)
	assert.Empty(t, scannerFindings(rep.Findings))
	assert.Empty(t, ofCode(rep.Findings, CodeScannerUnavailable))
}

func TestPresetMemberIsReplacedByAnEntryWithItsName(t *testing.T) {
	fakeBin(t, "agnix", "echo '"+sarifFor("AGX1", "error", "from the preset")+"'\n")
	own := fakeBin(t, "own-agnix", "echo '"+sarifFor("AGX1", "error", "from the entry")+"'\n")
	p := policyProject(t, "\n[lint.scanner_policy]\npreset = \"baseline\"\n\n[[lint.external]]\nname = \"agnix\"\ncommand = [\""+own+"\"]\negress = false\ninputs = [\"rules\"]\n")
	findings := p.run(Options{})
	got := scannerFindings(findings)
	require.Len(t, got, 1, dump(findings))
	assert.Contains(t, got[0].Message, "from the entry")
}

func TestProfileEntryInheritsAndCannotWeakenEgress(t *testing.T) {
	own := fakeBin(t, "skill-scanner", "echo '"+sarifFor("C1", "error", "profile ran")+"'\n")
	t.Run("inherits inputs and egress", func(t *testing.T) {
		p := policyProject(t, "\n[[lint.external]]\nname = \"c\"\nprofile = \"cisco-skill-scanner\"\ncommand = [\""+own+"\", \"{skill_dirs}\"]\n")
		findings := p.run(Options{})
		require.Len(t, scannerFindings(findings), 1, dump(findings))
		assert.Empty(t, ofCode(findings, CodeScannerEgressUndeclared), "the profile declares egress")
	})
	t.Run("egress false on an egress profile is AR9E0", func(t *testing.T) {
		p := policyProject(t, "\n[[lint.external]]\nname = \"s\"\nprofile = \"snyk-agent-scan\"\negress = false\n")
		findings := p.run(Options{})
		bad := ofCode(findings, CodeScannerConfigInvalid)
		require.Len(t, bad, 1, dump(findings))
		assert.Contains(t, bad[0].Message, "contradicts profile")
		assert.Empty(t, scannerFindings(findings))
	})
	t.Run("unknown profile is AR9E0", func(t *testing.T) {
		p := policyProject(t, "\n[[lint.external]]\nname = \"s\"\nprofile = \"nope\"\ncommand = [\"x\"]\n")
		bad := ofCode(p.run(Options{}), CodeScannerConfigInvalid)
		require.Len(t, bad, 1)
		assert.Contains(t, bad[0].Message, `profile "nope"`)
	})
	t.Run("an egress profile is blocked without the flag and its deny-list applies", func(t *testing.T) {
		p := policyProject(t, "\n[[lint.external]]\nname = \"c\"\nprofile = \"cisco-skill-scanner\"\ncommand = [\"skill-scanner\", \"--llm-provider\", \"x\"]\n")
		blocked := ofCode(p.run(Options{}), CodeScannerEgressBlocked)
		require.Len(t, blocked, 1)
		assert.Contains(t, blocked[0].Message, "--llm-provider")
	})
}

func TestRequiredScannerIsAnErrorWhenItDidNotRun(t *testing.T) {
	tests := []struct {
		name, toml, want string
	}{
		{"missing binary", "\n[[lint.external]]\nname = \"ghost\"\ncommand = [\"ai-rulez-no-such-scanner\"]\negress = false\nrequired = true\n", "[ghost]"},
		{"required by policy", "\n[lint.scanner_policy]\nrequired = [\"ghost\"]\n\n[[lint.external]]\nname = \"ghost\"\ncommand = [\"ai-rulez-no-such-scanner\"]\negress = false\n", "[ghost]"},
		{"required but not configured", "\n[lint.scanner_policy]\nrequired = [\"nowhere\"]\n", "[nowhere]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			findings := policyProject(t, tt.toml).run(Options{})
			got := ofCode(findings, CodeScannerUnavailable)
			require.Len(t, got, 1, dump(findings))
			assert.Equal(t, SeverityError, got[0].Severity)
			assert.Contains(t, got[0].Message, tt.want)
			assert.True(t, Failed(findings, "error"))
		})
	}
}

func TestVersionRangeGatesTheRun(t *testing.T) {
	script := `if [ "$1" = "--version" ]; then echo "fake-scan version 1.4.2 (build 7)"; exit 0; fi` + "\n" +
		"echo '" + sarifFor("V1", "error", "ran") + "'\n"
	tests := []struct {
		name, rng string
		wantRun   bool
		wantMsg   string
	}{
		{"inside the range", ">=1.0.0, <2", true, ""},
		{"above the range", ">=2", false, "is version 1.4.2, which does not satisfy"},
		{"bad range", "banana", false, "is not a version range"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bin := fakeBin(t, "fake-scan", script)
			p := policyProject(t, "\n[[lint.external]]\nname = \"fake\"\ncommand = [\""+bin+"\", \"{stage}\"]\negress = false\ninputs = [\"rules\"]\nversion = \""+tt.rng+"\"\n")
			findings := p.run(Options{})
			if tt.wantRun {
				require.Len(t, scannerFindings(findings), 1, dump(findings))
				return
			}
			assert.Empty(t, scannerFindings(findings))
			var msgs []string
			for _, f := range findings {
				msgs = append(msgs, f.Message)
			}
			assert.Contains(t, strings.Join(msgs, "\n"), tt.wantMsg)
		})
	}
}

func TestAllowEgressListGatesEgressScanners(t *testing.T) {
	bin := fakeBin(t, "egress-scan", "echo '"+sarifFor("E1", "error", "ran")+"'\n")
	entry := "\n[[lint.external]]\nname = \"e\"\ncommand = [\"" + bin + "\", \"{stage}\"]\negress = true\ninputs = [\"rules\"]\n"
	tests := []struct {
		name    string
		policy  string
		allow   []string
		wantRun bool
		wantMsg string
	}{
		{"flag alone, no policy list", "", []string{"e"}, true, ""},
		{"no flag", "", nil, false, "--allow-egress=e"},
		{"flag and listed", "\n[lint.scanner_policy]\nallow_egress = [\"e\"]\n", []string{"e"}, true, ""},
		{"flag but not listed", "\n[lint.scanner_policy]\nallow_egress = [\"other\"]\n", []string{"e"}, false, "allow_egress does not list it"},
		{"empty list forbids every egress scanner", "\n[lint.scanner_policy]\nallow_egress = []\n", []string{"e"}, false, "allow_egress does not list it"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := policyProject(t, tt.policy+entry)
			findings := p.run(Options{AllowEgress: tt.allow})
			if tt.wantRun {
				require.Len(t, scannerFindings(findings), 1, dump(findings))
				assert.Empty(t, ofCode(findings, CodeScannerEgressBlocked))
				return
			}
			assert.Empty(t, scannerFindings(findings))
			blocked := ofCode(findings, CodeScannerEgressBlocked)
			require.Len(t, blocked, 1, dump(findings))
			assert.Contains(t, blocked[0].Message, tt.wantMsg)
		})
	}
}

func TestFailOnAppliesToScannerFindingsOnly(t *testing.T) {
	bin := fakeBin(t, "warn-scan", "echo '"+sarifFor("W1", "warning", "mild")+"'\n")
	entry := "\n[[lint.external]]\nname = \"w\"\ncommand = [\"" + bin + "\", \"{stage}\"]\negress = false\ninputs = [\"rules\"]\n"
	tests := []struct {
		name, policy string
		wantFailed   bool
	}{
		{"no policy: a warning does not fail", "", false},
		{"fail_on warning", "\n[lint.scanner_policy]\nfail_on = \"warning\"\n", true},
		{"fail_on error leaves a warning alone", "\n[lint.scanner_policy]\nfail_on = \"error\"\n", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			findings := policyProject(t, tt.policy+entry).run(Options{})
			require.Len(t, scannerFindings(findings), 1, dump(findings))
			assert.Equal(t, tt.wantFailed, Failed(findings, "error"))
		})
	}
	t.Run("an unrelated warning is not promoted", func(t *testing.T) {
		assert.False(t, Failed([]Finding{{Code: "AR201", Severity: SeverityWarning}}, "error"))
	})
}

func TestPolicyBaselinePath(t *testing.T) {
	bin := fakeBin(t, "bl-scan", "echo '"+sarifFor("B1", "error", "known")+"'\n")
	entry := "\n[[lint.external]]\nname = \"bl\"\ncommand = [\"" + bin + "\", \"{stage}\"]\negress = false\ninputs = [\"rules\"]\n"
	t.Run("writes and reads the configured file", func(t *testing.T) {
		p := policyProject(t, "\n[lint.scanner_policy]\nbaseline = \"ci/scan-baseline.json\"\n"+entry)
		p.run(Options{Scanner: ScannerOptions{WriteBaseline: true, Reason: "known fixture"}})
		assert.FileExists(t, filepath.Join(p.root, "ci", "scan-baseline.json"))
		findings := p.run(Options{})
		assert.Empty(t, scannerFindings(findings), "the baselined finding is accepted:\n%s", dump(findings))
	})
	for _, bad := range []string{"../elsewhere.json", "/etc/scan.json"} {
		t.Run("rejects "+bad, func(t *testing.T) {
			p := policyProject(t, "\n[lint.scanner_policy]\nbaseline = \""+bad+"\"\n"+entry)
			got := ofCode(p.run(Options{}), CodeScannerConfigInvalid)
			require.Len(t, got, 1)
			assert.Contains(t, got[0].Message, "inside the project")
		})
	}
}

func TestInvalidScannerPolicyValuesAreAR9E0(t *testing.T) {
	for _, body := range []string{"preset = \"loud\"", "fail_on = \"fatal\"", "isolation = \"maybe\""} {
		t.Run(body, func(t *testing.T) {
			p := policyProject(t, "\n[lint.scanner_policy]\n"+body+"\n")
			assert.NotEmpty(t, ofCode(p.run(Options{}), CodeScannerConfigInvalid))
			assert.NotEmpty(t, ValidateSettings(&config.LintConfig{ScannerPolicy: &config.LintScannerPolicy{Preset: "loud"}}))
		})
	}
}

func TestIsolationModes(t *testing.T) {
	bin := fakeBin(t, "iso-scan", "echo '"+sarifFor("I1", "note", "ran")+"'\n")
	entry := "\n[[lint.external]]\nname = \"iso\"\ncommand = [\"" + bin + "\", \"{stage}\"]\negress = false\ninputs = [\"rules\"]\n"
	none := sandbox.New("windows", func(string) (string, error) { return "", os.ErrNotExist })
	tests := []struct {
		name      string
		mode      string
		wantRun   bool
		wantNotes int
		wantFail  bool
	}{
		{"auto without a backend runs and notes it once", "auto", true, 1, false},
		{"unset behaves as auto", "", true, 1, false},
		{"none is silent", "none", true, 0, false},
		{"require refuses", "require", false, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prev := scannerSandbox
			scannerSandbox = none
			t.Cleanup(func() { scannerSandbox = prev })
			policy := ""
			if tt.mode != "" {
				policy = "\n[lint.scanner_policy]\nisolation = \"" + tt.mode + "\"\n"
			}
			findings := policyProject(t, policy+entry).run(Options{})
			assert.Equal(t, tt.wantRun, len(scannerFindings(findings)) == 1, dump(findings))
			assert.Len(t, ofCode(findings, CodeScannerNoIsolation), tt.wantNotes, dump(findings))
			failed := ofCode(findings, CodeScannerRunFailed)
			assert.Equal(t, tt.wantFail, len(failed) == 1, dump(findings))
			if tt.wantFail {
				assert.Contains(t, failed[0].Message, "isolation")
			}
		})
	}
}

func TestRequireIsolationRefusesAScannerInTheProjectRoot(t *testing.T) {
	bin := fakeBin(t, "root-scan", "echo '"+sarifFor("R1", "note", "ran")+"'\n")
	p := policyProject(t, "\n[lint.scanner_policy]\nisolation = \"require\"\n\n[[lint.external]]\nname = \"root\"\ncommand = [\""+bin+"\"]\negress = false\n")
	findings := p.run(Options{})
	failed := ofCode(findings, CodeScannerRunFailed)
	require.Len(t, failed, 1, dump(findings))
	assert.Contains(t, failed[0].Message, "project root")
	assert.Empty(t, scannerFindings(findings))
}

// The staged scanner below tries to open a socket and to write outside its
// scratch directory; under real isolation it can do neither.
func TestStagedScannerIsConfinedByTheRealSandbox(t *testing.T) {
	if err := scannerSandbox.Check(context.Background()); err != nil {
		t.Skipf("no usable process isolation: %v", err)
	}
	if scannerSandbox.Backend() == sandbox.BackendUnshare {
		t.Skip("unshare confines the network only")
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() }) //nolint:errcheck // test
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close() //nolint:errcheck // test
		}
	}()
	port := ln.Addr().(*net.TCPAddr).Port
	outside := filepath.Join(t.TempDir(), "escaped")
	script := fmt.Sprintf(`#!/bin/bash
if [ "$1" = "--version" ]; then echo 1.0.0; exit 0; fi
if (exec 3<>/dev/tcp/127.0.0.1/%d) 2>/dev/null; then net=open; else net=denied; fi
if (echo x > %s) 2>/dev/null; then write=escaped; else write=denied; fi
if (echo x > "$TMPDIR/inside") 2>/dev/null; then scratch=ok; else scratch=denied; fi
printf '{"version":"2.1.0","runs":[{"results":[{"ruleId":"ISO","level":"note","message":{"text":"net=%%s write=%%s scratch=%%s"}}]}]}' "$net" "$write" "$scratch"
`, port, outside)
	bin := filepath.Join(t.TempDir(), "probe-scan")
	require.NoError(t, os.WriteFile(bin, []byte(script), 0o755)) //nolint:gosec // a test script
	entry := "\n[[lint.external]]\nname = \"probe\"\ncommand = [\"" + bin + "\", \"{stage}\"]\negress = false\ninputs = [\"rules\"]\n"
	messageOf := func(policy string) string {
		findings := policyProject(t, policy+entry).run(Options{})
		got := scannerFindings(findings)
		require.Len(t, got, 1, dump(findings))
		return got[0].Message
	}
	// Confined: no socket, no write outside, scratch still writable.
	assert.Contains(t, messageOf("\n[lint.scanner_policy]\nisolation = \"require\"\n"), "net=denied write=denied scratch=ok")
	assert.NoFileExists(t, outside)
	// Control: with isolation off the same scanner reaches both.
	assert.Contains(t, messageOf("\n[lint.scanner_policy]\nisolation = \"none\"\n"), "net=open write=escaped scratch=ok")
	assert.FileExists(t, outside)
}

func TestEgressBannerNamesWhatIsSent(t *testing.T) {
	// The profile entry runs the fake binary of the same name; its output is not
	// the adapter's shape, so only the banner (printed before the run) matters.
	fakeBin(t, "snyk-agent-scan", "echo '{}'\n")
	var buf bytes.Buffer
	log := logger.New(&buf, slog.LevelDebug)
	p := policyProject(t, "\n[[lint.external]]\nname = \"snyk\"\nprofile = \"snyk-agent-scan\"\n")
	cfg := loadNoRemote(t, p.root)
	tree, err := LoadTree(p.root)
	require.NoError(t, err)
	_, err = RunWith(cfg, tree, Options{External: true, AllowEgress: []string{"snyk"}}, WithHost(ambient.Host{Log: log}))
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "with egress")
	assert.Contains(t, out, "MCP server configuration and tool descriptions")
	assert.Contains(t, out, "--allow-egress=snyk")
}

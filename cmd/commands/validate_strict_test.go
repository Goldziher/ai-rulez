package commands

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
)

func TestCheckStrictFlags(t *testing.T) {
	tests := []struct {
		name    string
		strict  bool
		format  string
		failOn  string
		wantErr bool
	}{
		{"plain validate", false, "", "", false},
		{"strict defaults", true, "", "", false},
		{"strict json", true, "json", "warning", false},
		{"format without strict", false, "json", "", true},
		{"fail-on without strict", false, "", "error", true},
		{"unknown format", true, "xml", "", true},
		{"unknown fail-on", true, "", "loud", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldS, oldF, oldO := validateStrict, validateFormat, validateFailOn
			t.Cleanup(func() { validateStrict, validateFormat, validateFailOn = oldS, oldF, oldO })
			validateStrict, validateFormat, validateFailOn = tt.strict, tt.format, tt.failOn
			if err := checkStrictFlags(); (err != nil) != tt.wantErr {
				t.Errorf("checkStrictFlags() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestFailOnPrecedence(t *testing.T) {
	t.Cleanup(func() { validateFailOn = "" })
	validateFailOn = ""
	if got := failOnFor(nil); got != "error" {
		t.Errorf("default = %q, want error", got)
	}
	validateFailOn = "none"
	if got := failOnFor(nil); got != "none" {
		t.Errorf("flag = %q, want none", got)
	}
}

func TestCheckStrictFlags_External(t *testing.T) {
	t.Cleanup(func() { validateStrict, validateExtern = false, false })
	validateStrict, validateExtern = false, true
	if err := checkStrictFlags(); err == nil {
		t.Error("--external without --strict must be rejected")
	}
	validateStrict = true
	if err := checkStrictFlags(); err != nil {
		t.Errorf("--external with --strict must be accepted: %v", err)
	}
}

func TestCheckStrictFlags_AllowEgress(t *testing.T) {
	t.Cleanup(func() { validateStrict, validateExtern, validateAllowEgress = false, false, nil })
	validateStrict, validateExtern, validateAllowEgress = true, false, []string{"snyk"}
	if err := checkStrictFlags(); err == nil {
		t.Error("--allow-egress without --external must be rejected")
	}
	validateExtern = true
	if err := checkStrictFlags(); err != nil {
		t.Errorf("--allow-egress with --external must be accepted: %v", err)
	}
	for _, cmd := range []string{"validate", "scan"} {
		c := ValidateCmd
		if cmd == "scan" {
			c = ScanCmd
		}
		if c.Flags().Lookup("allow-egress") == nil {
			t.Errorf("%s is missing --allow-egress", cmd)
		}
	}
}

func TestScanCommand(t *testing.T) {
	for _, name := range []string{"recursive", "external", "format", "fail-on", "no-local", "config-dir"} {
		if ScanCmd.Flags().Lookup(name) == nil {
			t.Errorf("scan is missing --%s", name)
		}
	}
}

func TestEnforceScanImports(t *testing.T) {
	cfg := importingConfig(t, "error")
	if err := enforceScanImports(cfg); err == nil {
		t.Fatal("an imported secret must stop generation at level error")
	}
	cfg = importingConfig(t, "warn")
	if err := enforceScanImports(cfg); err != nil {
		t.Fatalf("level warn must only log: %v", err)
	}
	cfg = importingConfig(t, "")
	if err := enforceScanImports(cfg); err == nil {
		t.Fatal("scanning is on by default: an imported secret is an error-level finding and must stop generation")
	}
	cfg = importingConfig(t, "off")
	if err := enforceScanImports(cfg); err != nil {
		t.Fatalf("scan_imports = \"off\" opts out: %v", err)
	}
}

func TestReportStrictJudgesEachRootByItsOwnThreshold(t *testing.T) {
	oldF, oldO := validateFormat, validateFailOn
	t.Cleanup(func() { validateFormat, validateFailOn = oldF, oldO })
	validateFormat, validateFailOn = "text", ""
	failing := &lint.Report{Findings: []lint.Finding{{Code: lint.CodeReferenceUnknown, Severity: lint.SeverityError}}}
	top := &config.Config{}
	nested := &config.Config{Lint: &config.LintConfig{FailOn: "none"}}

	got := reportStrict([]*lint.Report{failing, {}}, []*config.Config{top, nested})

	if got != exitStrictFindings {
		t.Errorf("exit = %d, want %d: a nested fail_on=none must not silence the top root", got, exitStrictFindings)
	}
	if got := reportStrict([]*lint.Report{{}, failing}, []*config.Config{top, nested}); got != 0 {
		t.Errorf("exit = %d, want 0: the nested root opted out of failing", got)
	}
}

func TestApplyRepoRoot(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	oldRoot, oldCache := validateRepoRoot, strictTreeCache
	t.Cleanup(func() { validateRepoRoot, strictTreeCache = oldRoot, oldCache })

	dir := t.TempDir()
	if out, err := gitutil.CommandNoContext(dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	t.Setenv(repoRootEnv, dir)
	validateRepoRoot = ""
	if err := applyRepoRoot(); err != nil {
		t.Fatalf("env root: %v", err)
	}
	if strictTreeCache.Root != dir {
		t.Fatalf("env root not applied: %q", strictTreeCache.Root)
	}

	validateRepoRoot = dir + "/missing"
	if err := applyRepoRoot(); err == nil {
		t.Fatal("a missing --repo-root must be an error")
	}
}

func TestApplyRepoRoot_RefusesADirectoryOutsideGit(t *testing.T) {
	// Arrange: a plain directory that would otherwise be walked file by file.
	oldRoot, oldCache := validateRepoRoot, strictTreeCache
	t.Cleanup(func() { validateRepoRoot, strictTreeCache = oldRoot, oldCache })
	t.Setenv(repoRootEnv, "")
	strictTreeCache = lint.Loader{}
	validateRepoRoot = t.TempDir()

	// Act
	err := applyRepoRoot()

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not inside a git repository")
	assert.Empty(t, strictTreeCache.Root, "the loader keeps its default")
}

// scan always runs the strict checks and has no --strict flag, so none of its
// flag descriptions may tell the user to pass one.
func TestScanFlagDescriptionsDoNotMentionStrict(t *testing.T) {
	require.Nil(t, ScanCmd.Flags().Lookup("strict"))
	ScanCmd.Flags().VisitAll(func(f *pflag.Flag) {
		assert.NotContains(t, f.Usage, "--strict", "scan --%s", f.Name)
	})
	for _, name := range []string{"baseline", "update-baseline", "strict-baseline", "since", "changed"} {
		f := ScanCmd.Flags().Lookup(name)
		require.NotNil(t, f, name)
		assert.Regexp(t, `^[A-Z]`, f.Usage, "scan --%s starts a sentence", name)
		v := ValidateCmd.Flags().Lookup(name)
		require.NotNil(t, v, name)
		assert.True(t, strings.HasPrefix(v.Usage, "With --strict, "), "validate --%s", name)
	}
}

// pflag turns a back-quoted word in a usage string into the value placeholder,
// so "see `ai-rulez roles list`" would print as "--role ai-rulez roles list".
// Quote with single quotes instead. Every command is checked, not only validate.
func TestNoFlagUsageHasBackticks(t *testing.T) {
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		c.Flags().VisitAll(func(f *pflag.Flag) {
			assert.NotContains(t, f.Usage, "`", "%s --%s", c.CommandPath(), f.Name)
		})
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(RootCmd)
}

func TestScanLongTextDoesNotPromiseNothingRuns(t *testing.T) {
	assert.NotContains(t, ScanCmd.Long, "Nothing is fetched or executed.", "--external runs programs")
	assert.Contains(t, ScanCmd.Long, "--external")
}

func TestCheckAllowEgressRejectsUndeclaredScanner(t *testing.T) {
	cfg := &config.Config{Lint: &config.LintConfig{External: []config.LintExternal{{Name: "semgrep"}}}}
	tests := []struct {
		name    string
		allow   []string
		wantErr bool
	}{
		{"declared", []string{"semgrep"}, false},
		{"typo", []string{"semgerp"}, true},
		{"none", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			old := validateAllowEgress
			t.Cleanup(func() { validateAllowEgress = old })
			validateAllowEgress = tt.allow
			err := checkAllowEgress(cfg)
			assert.Equal(t, tt.wantErr, err != nil)
		})
	}
}

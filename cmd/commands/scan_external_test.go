package commands

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
)

func TestScannerOptionsValidatesFlagCombinations(t *testing.T) {
	tests := []struct {
		name    string
		extern  bool
		write   bool
		reason  string
		show    bool
		wantErr string
		noToday bool
	}{
		{name: "nothing set", extern: false, noToday: true},
		{name: "external alone", extern: true},
		{name: "write with reason", extern: true, write: true, reason: "why"},
		{name: "write without reason", extern: true, write: true, wantErr: "needs --reason"},
		{name: "reason without write", extern: true, reason: "why", wantErr: "only applies with --write-baseline"},
		{name: "write without external", write: true, reason: "why", wantErr: "require --external"},
		{name: "show-suppressed without external", show: true, wantErr: "require --external"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			oldE, oldW, oldR, oldS, oldT := validateExtern, scanWriteBaseline, scanReason, scanShowSuppressed, validateToday
			t.Cleanup(func() {
				validateExtern, scanWriteBaseline, scanReason, scanShowSuppressed, validateToday = oldE, oldW, oldR, oldS, oldT
			})
			validateExtern, scanWriteBaseline, scanReason, scanShowSuppressed, validateToday = tt.extern, tt.write, tt.reason, tt.show, "2026-10-06"
			// Act
			opts, err := scannerOptions()
			// Assert
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tt.noToday {
				return
			}
			if opts.WriteBaseline != tt.write || opts.Reason != tt.reason || opts.Today != "2026-10-06" {
				t.Errorf("opts = %+v", opts)
			}
		})
	}
}

func TestScannerFlagsNeedExternalAndDryRunIsScoped(t *testing.T) {
	restore := func() {
		validateExtern, scanNoCache, validateDryRun, validateStrict = false, false, false, false
	}
	t.Cleanup(restore)

	t.Run("--no-scan-cache needs --external", func(t *testing.T) {
		restore()
		scanNoCache = true
		_, err := scannerOptions()
		if err == nil || !strings.Contains(err.Error(), "--no-scan-cache require --external") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("--no-scan-cache is passed on", func(t *testing.T) {
		restore()
		validateExtern, scanNoCache = true, true
		opts, err := scannerOptions()
		if err != nil || !opts.NoCache {
			t.Fatalf("opts = %+v, err = %v", opts, err)
		}
		if got := withScannerCache(opts, &config.Config{ConfigDir: t.TempDir()}); got.Cache != nil {
			t.Error("--no-scan-cache must not attach the cache")
		}
	})
	t.Run("--dry-run without --fix needs --external", func(t *testing.T) {
		restore()
		validateStrict, validateDryRun = true, true
		if err := checkFlagCombinations(); err == nil || !strings.Contains(err.Error(), "--external") {
			t.Fatalf("err = %v", err)
		}
		validateExtern = true
		if err := checkFlagCombinations(); err != nil {
			t.Fatalf("--dry-run with --external must be accepted: %v", err)
		}
	})
	t.Run("--dry-run sends the plan to stderr under a structured format", func(t *testing.T) {
		restore()
		validateExtern, validateDryRun = true, true
		old := validateFormat
		t.Cleanup(func() { validateFormat = old })
		validateFormat = "json"
		opts, err := scannerOptions()
		if err != nil || !opts.DryRun || opts.Out != os.Stderr {
			t.Fatalf("opts = %+v, err = %v", opts, err)
		}
		validateFormat = ""
		if opts, _ = scannerOptions(); opts.Out != os.Stdout {
			t.Errorf("text plan must go to stdout")
		}
	})
}

func TestScanDryRunPrintsThePlanAndRunsNothing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake scanners are POSIX shell scripts")
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	marker := filepath.Join(t.TempDir(), "ran")
	script := filepath.Join(t.TempDir(), "fake-scan")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ntouch '"+marker+"'\n"), 0o755); err != nil { //nolint:gosec // test script
		t.Fatal(err)
	}
	root := lockProject(t, "\n[[lint.external]]\nname = \"fake\"\ncommand = [\""+filepath.ToSlash(script)+"\", \"{stage}\"]\negress = false\ninputs = [\"rules\"]\n")
	t.Cleanup(func() { validateExtern, validateDryRun, validateStrict = false, false, false })
	cfg, err := config.LoadConfig(context.Background(), root, config.WithoutRemote())
	if err != nil {
		t.Fatal(err)
	}
	validateExtern, validateDryRun, validateStrict = true, true, true

	var code int
	stdout, _ := capture(t, func() {
		report, lerr := strictLint(t.Context(), cfg)
		if lerr != nil {
			t.Error(lerr)
			return
		}
		code = reportStrict([]*lint.Report{report}, []*config.Config{cfg})
	})

	if code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Error("the scanner was started by a dry run")
	}
	for _, want := range []string{"scanner fake (egress = false", "<stage>", ".ai-rulez/rules/style.md", "cache:"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("plan lacks %q:\n%s", want, stdout)
		}
	}
	if strings.Contains(stdout, "AR0") || strings.Contains(stdout, "finding") {
		t.Errorf("a dry run prints no report:\n%s", stdout)
	}
}

package commands

import "testing"

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
	if err := enforceScanImports(cfg); err != nil {
		t.Fatalf("scanning is off by default: %v", err)
	}
}

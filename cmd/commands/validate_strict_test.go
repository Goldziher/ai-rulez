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

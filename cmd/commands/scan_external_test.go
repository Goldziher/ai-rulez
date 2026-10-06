package commands

import (
	"strings"
	"testing"
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

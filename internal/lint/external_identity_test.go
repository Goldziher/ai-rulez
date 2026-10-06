package lint

import (
	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestScannerFingerprint_OwnFingerprintStaysDistinctPerOccurrence(t *testing.T) {
	// Arrange: a scanner whose primaryLocationLineHash is equal for two results of
	// one rule (the same line text in two places).
	r := &runner{}
	occurrence := map[string]int{}
	finding := externalFinding{Rule: "TOK", Message: "bad token", Fingerprint: "primaryLocationLineHash=abc:1"}

	// Act
	first := r.scannerFingerprint("scan", finding, "", 0, occurrence)
	second := r.scannerFingerprint("scan", finding, "", 0, occurrence)
	other := r.scannerFingerprint("scan", finding, "", 0, map[string]int{})

	// Assert
	assert.NotEqual(t, first, second, "equal scanner fingerprints must not collapse into one finding")
	assert.Equal(t, first, other, "the first occurrence is stable across runs")
}

func TestScannerDate_RejectsMalformedValues(t *testing.T) {
	tests := []struct {
		name, env, opt string
		wantEnv        bool
	}{
		{name: "valid env", env: "2030-01-02", wantEnv: true},
		{name: "garbage env", env: "9999"},
		{name: "not a date", env: "tomorrow"},
		{name: "valid option", opt: "2031-02-03"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(TodayEnv, tt.env)
			got := ScannerOptions{Today: tt.opt}.today(ambient.Host{})
			switch {
			case tt.opt != "":
				assert.Equal(t, tt.opt, got)
			case tt.wantEnv:
				assert.Equal(t, tt.env, got)
			default:
				assert.NotEqual(t, tt.env, got, "a malformed date must fall back to the clock")
				assert.Regexp(t, `^\d{4}-\d{2}-\d{2}$`, got)
			}
		})
	}
}

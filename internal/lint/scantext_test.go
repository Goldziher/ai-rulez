package lint

import "testing"

func TestScanTextIgnoresInlineSuppression(t *testing.T) {
	// Arrange
	text := "<!-- ai-rulez-lint-ignore AR001 -->\ntoken = \"ghp_" + "abcdefghijklmnopqrstuvwxyz0123456789\"\n"

	// Act
	findings := ScanText(".ai-rulez/rules/x.md", text)

	// Assert
	found := false
	for _, f := range findings {
		if f.Code == CodeSecretDetected {
			found = true
		}
	}
	if !found {
		t.Fatalf("secret not reported despite inline ignore: %+v", findings)
	}
}

func TestDetectSecret(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"github token", "ghp_" + "abcdefghijklmnopqrstuvwxyz0123456789", true},
		{"assignment", `api_key = "abcdefghij1234567890abcd"`, true},
		{"placeholder()", "your-key-here", false},
		{"plain", "debug", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, got := DetectSecret(tt.in); got != tt.want {
				t.Fatalf("DetectSecret(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

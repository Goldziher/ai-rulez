package lint

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRedactSecretsMasksEveryCredentialTheScanRecognises(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"aws key", "key AKIAIOSFODNN7EXAMPLE here", "key [REDACTED:AR001] here"},
		{"aws secret access key keeps the name", "export AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY\n", "export AWS_SECRET_ACCESS_KEY=[REDACTED:AR001]\n"},
		{"bare 40 characters stay", "id wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", "id wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"},
		{"github token", "t ghp_" + "abcdefghijklmnopqrstuvwxyz0123456789" + " end", "t [REDACTED:AR001] end"},
		{"generic assignment keeps the key name", `password = "abcd1234efgh5678ijkl9012"`, `password = "[REDACTED:AR001]"`},
		{"a placeholder is left alone", `api_key = "your-key-here-please"`, `api_key = "your-key-here-please"`},
		{"clean text", "Deploy the service to staging", "Deploy the service to staging"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RedactSecrets(tt.in)
			assert.Equal(t, tt.want, got)
			_, found := DetectSecret(got)
			assert.False(t, found, "nothing credential-shaped is left")
		})
	}
}

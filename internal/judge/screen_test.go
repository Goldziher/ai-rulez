package judge

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const screenKey = "zq-live-7f3a9c1d5e8b2046"

func TestNewRubricRefusesASecret(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		secrets []string
	}{
		{"a token shape", "Score 1. Use key sk-abcdefghijklmnopqrstuvwxyz0123 to verify.", nil},
		{"a key value pair", "Score 1. api_key=abcdefgh12345678 is the grader key.", nil},
		{"the configured key value", "Score 1. The grader key is " + screenKey + ".", []string{screenKey}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Act
			_, err := NewRubric("file:r.md", tc.text, tc.secrets...)

			// Assert
			require.ErrorContains(t, err, "secret")
			assert.NotContains(t, err.Error(), "abcdefghijkl")
			assert.NotContains(t, err.Error(), screenKey)
		})
	}
}

func TestPrepareScreensTheConfiguredKeyValue(t *testing.T) {
	data := []byte("# Agents\nDeploy with " + screenKey + " when asked.\n")
	tests := []struct {
		name       string
		opts       PrepareOptions
		wantSkip   string
		wantRedact bool
	}{
		{"refused", PrepareOptions{Secrets: []string{screenKey}}, SkipSecrets, false},
		{"masked with redact", PrepareOptions{Secrets: []string{screenKey}, Redact: true}, "", true},
		{"no key configured", PrepareOptions{}, "", false},
		{"a short value is not a key", PrepareOptions{Secrets: []string{"abc"}}, "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Act
			p := Prepare("AGENTS.md", data, tc.opts)

			// Assert
			assert.Equal(t, tc.wantSkip, p.Skip)
			assert.Equal(t, tc.wantRedact, p.Redacted)
			if len(tc.opts.Secrets) > 0 && len(tc.opts.Secrets[0]) > 8 {
				assert.NotContains(t, p.Content, screenKey)
				assert.NotContains(t, p.SkipReason, screenKey)
			}
		})
	}
}

func TestPrepareRedactsThePath(t *testing.T) {
	// Act
	p := Prepare("docs/AKIAIOSFODNN7EXAMPLE/"+screenKey+".md", []byte("Run tests.\n"), PrepareOptions{Secrets: []string{screenKey}})

	// Assert
	assert.NotContains(t, p.Path, "AKIAIOSFODNN7EXAMPLE")
	assert.NotContains(t, p.Path, screenKey)
	assert.Contains(t, p.Path, "[REDACTED]")
}

func TestBuildRequestCarriesOnlyTheRedactedPath(t *testing.T) {
	// Arrange
	r, err := BuiltinRubric(RubricInstructionQuality)
	require.NoError(t, err)
	p := Prepare("AKIAIOSFODNN7EXAMPLE.md", []byte("Run tests.\n"), PrepareOptions{})

	// Act
	req := BuildRequest(r, p)

	// Assert
	for _, m := range req.Messages {
		assert.NotContains(t, m.Content, "AKIAIOSFODNN7EXAMPLE")
	}
}

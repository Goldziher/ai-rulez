package telemetry

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolve_EnvironmentOptOutBeatsTheConsentRecordGates(t *testing.T) {
	tests := []struct {
		name        string
		env         func(string) string
		wantPaths   bool
		wantSession bool
	}{
		{name: "paths off in the environment", env: env(EnvIncludePaths, "0"), wantPaths: false, wantSession: true},
		{name: "session off in the environment", env: env(EnvIncludeSession, "0"), wantPaths: true, wantSession: false},
		{name: "both off in the environment", env: env(EnvIncludePaths, "0", EnvIncludeSession, "0")},
		{name: "no environment setting keeps the record gates", env: env(), wantPaths: true, wantSession: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			consent := record(consentEndpoint, "http/json", true, true)

			// Act
			s := Resolve(Layers{Consent: consent, Getenv: tt.env})

			// Assert
			assert.Equal(t, tt.wantPaths, s.IncludePaths)
			assert.Equal(t, tt.wantSession, s.IncludeSession)
			if !tt.wantPaths || !tt.wantSession {
				assert.False(t, s.ExportActive(), "the record no longer covers what would be sent")
			}
		})
	}
}

package contentlock

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

const verifierFileBody = "[[verifiers]]\nid = \"migrations-have-down\"\nrule = \"database\"\nseverity = \"error\"\nexclude = [\"legacy/**\"]\n" +
	"[verifiers.require.regex]\nregex = \"down\"\n\n[[verifiers]]\nid = \"second\"\nrule = \"database\"\n[verifiers.require.file_exists]\npath = \"x\"\n"

func TestComputePinsVerifierDeclarations(t *testing.T) {
	// Arrange
	f := newFixture(t)
	f.write("verifiers/db.toml", []byte(verifierFileBody), 0o644)
	f.cfg.Verifiers = []config.VerifierConfig{{Name: "readme", Type: "file_exists", Path: "README.md"}}

	// Act
	snap, err := Compute(f.cfg, Options{})

	// Assert
	require.NoError(t, err)
	got := map[string]string{}
	for _, it := range snap.Items {
		if it.Kind == KindVerifier {
			got[it.ID] = it.Path
		}
	}
	assert.Equal(t, map[string]string{"readme": "", "migrations-have-down": "verifiers/db.toml", "second": "verifiers/db.toml"}, got)
	assert.Empty(t, snap.Problems)
}

func TestVerifierPinChangesWhenACheckIsWeakened(t *testing.T) {
	tests := []struct {
		name string
		edit func(body string) string
	}{
		{"severity lowered", func(b string) string { return strings.Replace(b, `severity = "error"`, `severity = "warning"`, 1) }},
		{"exclude widened", func(b string) string { return strings.Replace(b, `"legacy/**"`, `"**"`, 1) }},
		{"regex relaxed", func(b string) string { return strings.Replace(b, `regex = "down"`, `regex = "d"`, 1) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			f := newFixture(t)
			f.write("verifiers/db.toml", []byte(verifierFileBody), 0o644)
			before := f.items()

			// Act
			f.write("verifiers/db.toml", []byte(tt.edit(verifierFileBody)), 0o644)
			after := f.items()

			// Assert
			assert.NotEqual(t, before["verifier:/migrations-have-down"], after["verifier:/migrations-have-down"])
			assert.Equal(t, before["verifier:/second"], after["verifier:/second"], "an untouched verifier keeps its pin")
		})
	}
}

func TestVerifierPinsAreReportedAsDriftByCompare(t *testing.T) {
	f := newFixture(t)
	f.write("verifiers/db.toml", []byte(verifierFileBody), 0o644)
	snap, err := Compute(f.cfg, Options{})
	require.NoError(t, err)
	var lock lockfile.File
	lock.Version = lockfile.Version
	Build(&lock, snap)

	f.write("verifiers/db.toml", []byte(strings.Replace(verifierFileBody, `severity = "error"`, `severity = "info"`, 1)), 0o644)
	now, err := Compute(f.cfg, Options{})
	require.NoError(t, err)
	diff := Compare(&lock, now)

	require.False(t, diff.InSync)
	require.Len(t, diff.Changes, 1)
	assert.Equal(t, KindVerifier, diff.Changes[0].Kind)
	assert.Equal(t, "migrations-have-down", diff.Changes[0].ID)
}

func TestVerifierSettingsArePinned(t *testing.T) {
	f := newFixture(t)
	before := f.items()
	f.cfg.VerifiersSettings = &config.VerifiersSettings{TrustExecFrom: []string{"shared"}}

	after := f.items()

	assert.NotContains(t, before, "settings:/verifiers-settings")
	assert.Contains(t, after, "settings:/verifiers-settings", "widening trust_exec_from must change the lock")
}

func TestUnparseableVerifierFileIsAProblemNotAnAbort(t *testing.T) {
	f := newFixture(t)
	f.write("verifiers/bad.toml", []byte("[[verifiers\n"), 0o644)

	snap, err := Compute(f.cfg, Options{})

	require.NoError(t, err)
	require.Len(t, snap.Problems, 1)
	assert.Contains(t, snap.Problems[0], "verifiers/bad.toml")
}

func TestLocalIncludePinCoversItsVerifiers(t *testing.T) {
	assert.True(t, localIncludeDirs["verifiers"], "a local include's verifiers/ directory must be part of its pin")
}

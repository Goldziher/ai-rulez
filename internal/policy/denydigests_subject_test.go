package policy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

func TestApplyDenyDigestsNamesTheSubject(t *testing.T) {
	// Arrange: one denied entry of each kind, including a served skill in a view.
	cfg := lockedConfig(t, badDigest, badDigest, badDigest, badDigest)
	path := filepath.Join(cfg.ConfigDir, lockfile.FileName)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	served := "\n[[served]]\nname = \"heavy\"\nsource = \"skills/heavy\"\nview = \"role:dev\"\ndigest = \"" + badDigest + "\"\n"
	require.NoError(t, os.WriteFile(path, append(data, served...), 0o600))
	res := Resolve([]Layer{layer("managed", Policy{Sources: Sources{DenyDigests: []string{badDigest}}})})

	// Act
	out := res.Apply(cfg).Outcome

	// Assert
	got := make([]config.PolicySubject, 0, len(out.Violations))
	for i := range out.Violations {
		require.NotNil(t, out.Violations[i].Subject, out.Violations[i].Message)
		got = append(got, *out.Violations[i].Subject)
	}
	assert.ElementsMatch(t, []config.PolicySubject{
		{Kind: lockfile.KindInclude, Name: "shared", Digest: badDigest},
		{Kind: lockfile.KindSkill, Name: "deploy", Digest: badDigest},
		{Kind: lockfile.KindSource, Name: "catalog", Digest: badDigest},
		{Kind: lockfile.KindServed, Name: "heavy", Domain: "role:dev", Digest: badDigest},
		{Kind: "rule", Name: "style", Digest: badDigest},
	}, got)
}

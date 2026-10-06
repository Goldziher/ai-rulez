package policy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

var (
	badDigest  = "sha256:" + strings.Repeat("bad0", 16)
	goodDigest = "sha256:" + strings.Repeat("0123", 16)
)

const denyLock = `version = 1

[[include]]
name = "shared"
source = "github.com/example-org/shared"
commit = "aaaa"
digest = "%s"

[[skill]]
name = "deploy"
source = "github.com/example-org/skills"
commit = "bbbb"
digest = "%s"

[[source]]
name = "catalog"
source = "github.com/example-org/catalog"
commit = "cccc"
digest = "%s"

[[item]]
kind = "rule"
id = "style"
path = "rules/style.md"
digest = "%s"
`

func lockedConfig(t *testing.T, include, skill, source, item string) *config.Config {
	t.Helper()
	cfg := testConfig(t, "name = \"x\"\n")
	body := sprintfAll(denyLock, include, skill, source, item)
	require.NoError(t, os.WriteFile(filepath.Join(cfg.ConfigDir, lockfile.FileName), []byte(body), 0o644))
	cfg.Includes = []config.IncludeConfig{{Name: "shared", Source: "github.com/example-org/shared"}, {Name: "other", Source: "github.com/example-org/other"}}
	cfg.InstalledSkills = []config.InstalledSkillConfig{{Name: "deploy", Source: "github.com/example-org/skills"}}
	cfg.SkillSources = []config.SkillSourceConfig{{Name: "catalog", URL: "github.com/example-org/catalog"}}
	return cfg
}

func sprintfAll(format string, args ...string) string {
	out := format
	for _, a := range args {
		out = strings.Replace(out, "%s", a, 1)
	}
	return out
}

func TestParseDenyDigests(t *testing.T) {
	// Arrange
	body := "policy_version = 1\n[sources]\ndeny_digests = [\"" + strings.ToUpper(badDigest[:7]) + badDigest[7:] + "\", \"" + badDigest + "\", \"" + goodDigest + "\"]\n"
	// Act
	_, p, err := Parse("p.toml", []byte(body))
	// Assert
	require.NoError(t, err)
	assert.Equal(t, []string{goodDigest, badDigest}, p.Sources.DenyDigests, "lower-cased, sorted, unique")
}

func TestParseDenyDigestsRejects(t *testing.T) {
	for _, bad := range []string{"", "abc", "sha256:xyz", "md5:" + strings.Repeat("a", 32), "sha256:" + strings.Repeat("a", 63)} {
		t.Run(bad, func(t *testing.T) {
			// Act
			_, _, err := Parse("p.toml", []byte("policy_version = 1\n[sources]\ndeny_digests = [\""+bad+"\"]\n"))
			// Assert
			require.Error(t, err)
			assert.Contains(t, err.Error(), "AR743")
			assert.Contains(t, err.Error(), "deny_digests")
		})
	}
}

func TestMergeDenyDigestsUnion(t *testing.T) {
	a := Policy{Sources: Sources{DenyDigests: []string{badDigest}}}
	b := Policy{Sources: Sources{DenyDigests: []string{goodDigest}}}
	got := Merge(a, b)
	assert.Equal(t, []string{goodDigest, badDigest}, got.Sources.DenyDigests)
	assert.Equal(t, got, Merge(b, a))
	assert.Equal(t, a.Sources.DenyDigests, Merge(a, Policy{}).Sources.DenyDigests)
}

func TestApplyDenyDigests(t *testing.T) {
	tests := []struct {
		name         string
		include      string
		skill        string
		source       string
		item         string
		wantIncludes []string
		wantSkills   int
		wantSources  int
		wantViol     int
	}{
		{"nothing denied", goodDigest, goodDigest, goodDigest, goodDigest, []string{"shared", "other"}, 1, 1, 0},
		{"a denied include is not loaded", badDigest, goodDigest, goodDigest, goodDigest, []string{"other"}, 1, 1, 1},
		{"a denied installed skill is not loaded", goodDigest, badDigest, goodDigest, goodDigest, []string{"shared", "other"}, 0, 1, 1},
		{"a denied skill source is not loaded", goodDigest, goodDigest, badDigest, goodDigest, []string{"shared", "other"}, 1, 0, 1},
		{"a denied authored item is reported", goodDigest, goodDigest, goodDigest, badDigest, []string{"shared", "other"}, 1, 1, 1},
		{"everything denied", badDigest, badDigest, badDigest, badDigest, []string{"other"}, 0, 0, 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := lockedConfig(t, tt.include, tt.skill, tt.source, tt.item)
			res := Resolve([]Layer{layer("managed", Policy{Sources: Sources{DenyDigests: []string{badDigest}}})})
			// Act
			out := res.Apply(cfg).Outcome
			// Assert
			var names []string
			for _, i := range cfg.Includes {
				names = append(names, i.Name)
			}
			assert.Equal(t, tt.wantIncludes, names)
			assert.Len(t, cfg.InstalledSkills, tt.wantSkills)
			assert.Len(t, cfg.SkillSources, tt.wantSources)
			assert.Len(t, out.Violations, tt.wantViol)
			for _, v := range out.Violations {
				assert.Equal(t, "AR747", v.Code)
				assert.Equal(t, "managed", v.Origin)
				assert.Contains(t, v.Message, badDigest)
			}
		})
	}
}

func TestApplyDenyDigestsWithoutALockOrConfigDirIsQuiet(t *testing.T) {
	// Arrange
	pol := Policy{Sources: Sources{DenyDigests: []string{badDigest}}}
	noLock := testConfig(t, "name = \"x\"\n")
	noDir := &config.Config{}
	// Act and Assert
	assert.Empty(t, Resolve([]Layer{layer("managed", pol)}).Apply(noLock).Outcome.Violations)
	assert.Empty(t, Resolve([]Layer{layer("managed", pol)}).Apply(noDir).Outcome.Violations)
}

func TestShowPolicyListsDeniedDigests(t *testing.T) {
	// Arrange
	res := Resolve([]Layer{layer("managed", Policy{Sources: Sources{DenyDigests: []string{badDigest}}})})
	// Act
	tree := res.Policy.Tree()
	// Assert
	assert.Equal(t, []string{badDigest}, tree["sources"].(map[string]any)["deny_digests"])
	assert.Equal(t, "managed", res.Provenance["sources.deny_digests"])
}

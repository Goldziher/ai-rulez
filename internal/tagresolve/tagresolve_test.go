package tagresolve

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/tagresolve/tagtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	shaA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	shaB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	shaC = "cccccccccccccccccccccccccccccccccccccccc"
)

func rawTags(names ...string) []RawTag {
	out := make([]RawTag, len(names))
	for i, n := range names {
		out[i] = RawTag{Name: n, Object: shaA, Commit: shaA}
	}
	return out
}

func TestParseLsRemote(t *testing.T) {
	out := strings.Join([]string{
		shaA + "\trefs/tags/v1.0.0",
		shaB + "\trefs/tags/v1.1.0",
		shaC + "\trefs/tags/v1.1.0^{}",
		shaA + "\trefs/heads/main",
		"garbage line",
		"zz\trefs/tags/bad-sha",
		shaA + "\trefs/tags/with\x01control",
	}, "\n")

	got := ParseLsRemote(out)

	assert.Equal(t, []RawTag{
		{Name: "v1.0.0", Object: shaA, Commit: shaA},
		{Name: "v1.1.0", Object: shaB, Commit: shaC},
	}, got)
	assert.False(t, got[0].Annotated())
	assert.Empty(t, got[0].TagObject())
	assert.True(t, got[1].Annotated())
	assert.Equal(t, shaB, got[1].TagObject())
}

func TestSelect(t *testing.T) {
	tests := []struct {
		name       string
		tags       []string
		spec       Spec
		wantTag    string
		wantLatest string
		wantCode   string
		wantMsg    string
	}{
		{name: "caret picks the highest compatible", tags: []string{"v1.2.0", "v1.2.4", "v1.3.1", "v2.0.0"}, spec: Spec{Constraint: "^1.2"}, wantTag: "v1.3.1", wantLatest: "v2.0.0"},
		{name: "tilde stays inside the minor", tags: []string{"v2.1.0", "v2.1.3", "v2.2.0"}, spec: Spec{Constraint: "~2.1.0"}, wantTag: "v2.1.3", wantLatest: "v2.2.0"},
		{name: "range", tags: []string{"1.3.0", "1.4.0", "1.9.0", "2.0.0"}, spec: Spec{Constraint: ">=1.4.0 <2.0.0"}, wantTag: "1.9.0", wantLatest: "2.0.0"},
		{name: "non-semver tags are ignored", tags: []string{"latest", "stable", "v1.0.0", "release-2", "v1.0"}, spec: Spec{Constraint: "^1"}, wantTag: "v1.0.0", wantLatest: "v1.0.0"},
		{name: "prereleases are skipped by default", tags: []string{"v1.2.0", "v1.3.0-rc.1"}, spec: Spec{Constraint: "^1.2"}, wantTag: "v1.2.0", wantLatest: "v1.2.0"},
		{name: "include_prerelease admits them", tags: []string{"v1.2.0", "v1.3.0-rc.1"}, spec: Spec{Constraint: "^1.2", IncludePrerelease: true}, wantTag: "v1.3.0-rc.1", wantLatest: "v1.3.0-rc.1"},
		{name: "a constraint naming a prerelease admits that tuple", tags: []string{"v1.2.0", "v1.3.0-rc.1", "v1.3.0-rc.2"}, spec: Spec{Constraint: ">=1.3.0-rc.1 <2"}, wantTag: "v1.3.0-rc.2", wantLatest: "v1.2.0"},
		{name: "prefixed monorepo tags", tags: []string{"deploy/v2.1.3", "deploy/v2.2.0", "lint/v9.0.0", "v7.0.0"}, spec: Spec{Constraint: "~2.1.0", TagPrefix: "deploy/v"}, wantTag: "deploy/v2.1.3", wantLatest: "deploy/v2.2.0"},
		{name: "with and without v name one version: the name sorts first", tags: []string{"v1.2.3", "1.2.3"}, spec: Spec{Constraint: "1.2.3"}, wantTag: "1.2.3", wantLatest: "1.2.3"},
		{name: "numeric precedence, not lexical", tags: []string{"v1.9.0", "v1.10.0", "v1.2.0"}, spec: Spec{Constraint: "^1"}, wantTag: "v1.10.0", wantLatest: "v1.10.0"},
		{name: "unsatisfiable names the nearest tags", tags: []string{"v1.0.0", "v2.0.0", "v2.1.0", "v2.2.0"}, spec: Spec{Constraint: "^3"}, wantCode: CodeUnsatisfiable, wantMsg: `"v2.2.0", "v2.1.0", "v2.0.0"`},
		{name: "no semver tags at all", tags: []string{"latest", "nightly"}, spec: Spec{Constraint: "^1"}, wantCode: CodeUnsatisfiable, wantMsg: "pin a commit SHA"},
		{name: "only prereleases hint at the opt-in", tags: []string{"v1.0.0-rc.1"}, spec: Spec{Constraint: "^1"}, wantCode: CodeUnsatisfiable, wantMsg: "include_prerelease"},
		{name: "no tags", tags: nil, spec: Spec{Constraint: "^1"}, wantCode: CodeUnsatisfiable, wantMsg: "no semantic version tags"},
		{name: "prefix with no matching tags", tags: []string{"v1.0.0"}, spec: Spec{Constraint: "^1", TagPrefix: "x/"}, wantCode: CodeUnsatisfiable, wantMsg: `with prefix "x/"`},
		{name: "a bad constraint is AR731", tags: []string{"v1.0.0"}, spec: Spec{Constraint: "^^1"}, wantCode: CodeConstraintBad},
		{name: "an empty constraint is AR731", tags: []string{"v1.0.0"}, spec: Spec{Constraint: ""}, wantCode: CodeConstraintBad},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			sel, err := Select(rawTags(tt.tags...), tt.spec)

			// Assert
			if tt.wantCode != "" {
				require.Error(t, err)
				var coded *Error
				require.True(t, errors.As(err, &coded), err.Error())
				assert.Equal(t, tt.wantCode, coded.Code)
				assert.Contains(t, err.Error(), tt.wantMsg)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantTag, sel.Chosen.Tag.Name)
			assert.Equal(t, tt.wantLatest, sel.Latest.Tag.Name)
		})
	}
}

func TestSelectReportsTwoTagsForOneVersion(t *testing.T) {
	sel, err := Select(rawTags("v1.2.3", "1.2.3"), Spec{Constraint: "^1"})

	require.NoError(t, err)
	require.Len(t, sel.Notes, 1)
	assert.Contains(t, sel.Notes[0], `"1.2.3" and "v1.2.3"`)
}

func TestSelectIsMonotone(t *testing.T) {
	// Adding a tag below the chosen one never changes the result.
	base := rawTags("v1.2.0", "v1.4.0", "v1.3.0")
	want, err := Select(base, Spec{Constraint: "^1"})
	require.NoError(t, err)

	for _, extra := range []string{"v1.0.0", "v0.9.0", "v1.3.9", "foo", "v1.4.0-rc.1"} {
		got, err := Select(append(rawTags(extra), base...), Spec{Constraint: "^1"})
		require.NoError(t, err)
		assert.Equal(t, want.Chosen.Tag.Name, got.Chosen.Tag.Name, "after adding %s", extra)
	}
}

func TestCheck(t *testing.T) {
	tags := []RawTag{{Name: "v1.0.0", Object: shaB, Commit: shaA}}
	tests := []struct {
		name   string
		tag    string
		commit string
		want   Status
	}{
		{"unchanged", "v1.0.0", shaA, StatusOK},
		{"moved to another commit", "v1.0.0", shaC, StatusMoved},
		{"deleted", "v9.9.9", shaA, StatusMissing},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := Check(tags, tt.tag, tt.commit)
			assert.Equal(t, tt.want, got)
		})
	}
}

// localRunner runs git against a file:// remote the way the callers do.
func localRunner(ctx context.Context, args ...string) (string, error) {
	cmd := gitutil.Command(ctx, "", append(gitutil.HardenedConfig(), args...)...)
	cmd.Env = gitutil.HardenedEnv(os.Environ())
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return "", errors.New(string(ee.Stderr))
		}
		return "", err
	}
	return string(out), nil
}

func TestListTagsAgainstALocalRemote(t *testing.T) {
	// Arrange: lightweight, annotated, non-semver and prefixed tags.
	repo := tagtest.New(t)
	repo.Write("a.txt", "1\n")
	repo.Commit("one")
	repo.Tag("v1.0.0")
	repo.Write("a.txt", "2\n")
	repo.Commit("two")
	repo.AnnotatedTag("v1.1.0")
	repo.Tag("latest")
	repo.Tag("deploy/v2.0.0")

	// Act
	tags, err := ListTags(context.Background(), localRunner, repo.URL)

	// Assert
	require.NoError(t, err)
	byName := map[string]RawTag{}
	for _, tg := range tags {
		byName[tg.Name] = tg
	}
	assert.Len(t, tags, 4)
	assert.False(t, byName["v1.0.0"].Annotated())
	assert.Equal(t, repo.TagCommit("v1.0.0"), byName["v1.0.0"].Commit)
	assert.True(t, byName["v1.1.0"].Annotated(), "an annotated tag keeps its tag object")
	assert.Equal(t, repo.TagObject("v1.1.0"), byName["v1.1.0"].Object)
	assert.Equal(t, repo.TagCommit("v1.1.0"), byName["v1.1.0"].Commit, "and resolves to the peeled commit")

	sel, err := Select(tags, Spec{Constraint: "^1"})
	require.NoError(t, err)
	assert.Equal(t, "v1.1.0", sel.Chosen.Tag.Name)
	assert.Equal(t, repo.TagCommit("v1.1.0"), sel.Chosen.Tag.Commit)
}

func TestCheckDetectsAMovedAndADeletedTagOnARemote(t *testing.T) {
	repo := tagtest.New(t)
	repo.Write("a.txt", "1\n")
	first := repo.Commit("one")
	repo.AnnotatedTag("v1.0.0")
	repo.Tag("v1.1.0")

	// The tag is force-pushed to a new commit and another tag disappears.
	repo.Write("a.txt", "evil\n")
	second := repo.Commit("two")
	repo.AnnotatedTag("v1.0.0")
	repo.DeleteTag("v1.1.0")
	tags, err := ListTags(context.Background(), localRunner, repo.URL)
	require.NoError(t, err)

	moved, now := Check(tags, "v1.0.0", first)
	missing, _ := Check(tags, "v1.1.0", first)

	assert.Equal(t, StatusMoved, moved)
	assert.Equal(t, second, now.Commit)
	assert.Equal(t, StatusMissing, missing)
	assert.Contains(t, MovedError("v1.0.0", first, second).Error(), CodeTagMoved)
}

package tagresolve

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

func tagsAt(commits map[string]string) []RawTag {
	var out []RawTag
	for name, sha := range commits {
		out = append(out, RawTag{Name: name, Object: sha, Commit: sha})
	}
	return out
}

func TestEvaluate(t *testing.T) {
	a, b, c := strings.Repeat("a", 40), strings.Repeat("b", 40), strings.Repeat("c", 40)
	want := lockfile.Want{Kind: "include", Name: "shared", Source: "https://x/y", Ref: "^1.2", Constraint: "^1.2"}
	pinned := func(tag, commit string) *lockfile.Entry {
		return &lockfile.Entry{Name: "shared", Source: "https://x/y", Ref: "^1.2", Tag: tag, Commit: commit}
	}
	tests := []struct {
		name       string
		entry      *lockfile.Entry
		tags       map[string]string
		wantStatus string
		wantCode   string
		wantMajor  bool
		allowed    string
	}{
		{"up to date", pinned("v1.2.4", a), map[string]string{"v1.2.4": a}, StatusUpToDate, "", false, "v1.2.4"},
		{"a newer allowed tag", pinned("v1.2.4", a), map[string]string{"v1.2.4": a, "v1.3.1": b}, StatusUpdatable, "", false, "v1.3.1"},
		{"a newer major exists", pinned("v1.2.4", a), map[string]string{"v1.2.4": a, "v2.0.0": b}, StatusUpToDate, "", true, "v1.2.4"},
		{"not locked", nil, map[string]string{"v1.2.4": a}, StatusNotLocked, "", false, "v1.2.4"},
		{"a moved tag", pinned("v1.2.4", a), map[string]string{"v1.2.4": c, "v1.3.0": b}, StatusTagMoved, "AR732", false, "v1.3.0"},
		{"a deleted tag", pinned("v1.2.4", a), map[string]string{"v1.3.0": b}, StatusTagMissing, "AR735", false, "v1.3.0"},
		{"nothing satisfies", pinned("v1.2.4", a), map[string]string{"v2.0.0": b}, StatusUnsatisfied, "AR730", false, ""},
		{"a prerelease tag is ignored", pinned("v1.3.0", a), map[string]string{"v1.3.0": a, "v1.4.0-x": b}, StatusUpToDate, "", false, "v1.3.0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row := Evaluate("include", "shared", want, tt.entry, tagsAt(tt.tags))

			assert.Equal(t, tt.wantStatus, row.Status)
			assert.Equal(t, tt.wantCode, row.Code)
			assert.Equal(t, tt.wantMajor, row.MajorAvailable)
			if tt.allowed != "" {
				require.NotNil(t, row.Allowed)
				assert.Equal(t, tt.allowed, row.Allowed.Tag)
			}
		})
	}
}

func TestEvaluateReportsALowerOnlyTag(t *testing.T) {
	a, b := strings.Repeat("a", 40), strings.Repeat("b", 40)
	want := lockfile.Want{Kind: "include", Name: "s", Source: "x", Ref: "~1.2.0", Constraint: "~1.2.0"}
	entry := &lockfile.Entry{Ref: "~1.2.0", Source: "x", Tag: "v1.2.9", Commit: a}
	row := Evaluate("include", "s", want, entry, tagsAt(map[string]string{"v1.2.9": a, "v1.2.3": b}))
	assert.Equal(t, StatusUpToDate, row.Status, "the highest allowed tag is the pin")

	// The pinned tag was deleted and only lower ones remain: the pin is missing, and the lower tag is not offered silently.
	row = Evaluate("include", "s", want, entry, tagsAt(map[string]string{"v1.2.3": b}))
	assert.Equal(t, StatusTagMissing, row.Status)
}

func TestReportIsSortedCountedAndRendered(t *testing.T) {
	rows := []Row{
		{Kind: "skill", Name: "deploy", Constraint: "~2.1.0", Status: StatusUpToDate, Locked: &TagRef{Tag: "v2.1.3"}, Allowed: &TagRef{Tag: "v2.1.3"}, Latest: &TagRef{Tag: "v2.2.0"}},
		{Kind: "include", Name: "shared", Constraint: "^1.2", Status: StatusUpdatable, Locked: &TagRef{Tag: "v1.2.4"}, Allowed: &TagRef{Tag: "v1.3.1"}, Latest: &TagRef{Tag: "v2.0.0"}, MajorAvailable: true},
		{Kind: "include", Name: "bad", Constraint: "^9", Status: StatusUnsatisfied, Code: "AR730", Note: "no tag"},
	}

	rep := NewReport(rows)
	var text, js bytes.Buffer
	require.NoError(t, rep.WriteText(&text))
	require.NoError(t, rep.WriteJSON(&js))

	assert.Equal(t, []string{"bad", "shared", "deploy"}, []string{rep.Sources[0].Name, rep.Sources[1].Name, rep.Sources[2].Name})
	assert.Equal(t, Summary{Total: 3, Updatable: 1, MajorAvailable: 1, Errors: 1}, rep.Summary)
	assert.True(t, rep.Failing())
	assert.Contains(t, text.String(), "SOURCE")
	assert.Contains(t, text.String(), "3 source(s): 1 updatable, 1 with a newer major, 0 moved tag(s), 1 error(s)")
	assert.Contains(t, js.String(), `"schema_version": 1`)
	assert.Contains(t, js.String(), `"status": "updatable"`)

	var again bytes.Buffer
	require.NoError(t, NewReport(rows).WriteJSON(&again))
	assert.Equal(t, js.String(), again.String(), "deterministic")
}

func TestEmptyReport(t *testing.T) {
	var out bytes.Buffer
	require.NoError(t, NewReport(nil).WriteText(&out))
	assert.Contains(t, out.String(), "no source uses a version constraint")
}

func TestEvaluateLockedNonVersionTag(t *testing.T) {
	a, b := strings.Repeat("a", 40), strings.Repeat("b", 40)
	want := lockfile.Want{Kind: "include", Name: "s", Source: "x", Ref: "^1.0", Constraint: "^1.0"}
	entry := &lockfile.Entry{Ref: "^1.0", Source: "x", Tag: "release-candidate", Commit: a}

	row := Evaluate("include", "s", want, entry, tagsAt(map[string]string{"release-candidate": a, "v1.0.0": b}))

	assert.Equal(t, StatusLockedNonVersion, row.Status)
	assert.Contains(t, row.Note, "release-candidate")
	assert.Contains(t, row.Note, "--allow-downgrade")
}

func TestSelectLatestFallsBackToChosenWhenOnlyPrereleasesExist(t *testing.T) {
	a := strings.Repeat("a", 40)

	sel, err := Select(tagsAt(map[string]string{"v1.0.0-rc.1": a}), Spec{Constraint: "=1.0.0-rc.1"})

	require.NoError(t, err)
	assert.Equal(t, "v1.0.0-rc.1", sel.Chosen.Tag.Name)
	assert.Equal(t, "v1.0.0-rc.1", sel.Latest.Tag.Name)
	row := Evaluate("include", "s", lockfile.Want{Kind: "include", Name: "s", Source: "x", Constraint: "=1.0.0-rc.1"}, nil, tagsAt(map[string]string{"v1.0.0-rc.1": a}))
	assert.Equal(t, "v1.0.0-rc.1", row.Latest.Tag)
}

func TestTagMissingCountsAsAnError(t *testing.T) {
	rep := NewReport([]Row{{Kind: "include", Name: "s", Status: StatusTagMissing, Code: "AR735"}})

	assert.Equal(t, 1, rep.Summary.Errors)
	assert.True(t, rep.Failing())
}

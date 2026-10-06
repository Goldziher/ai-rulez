package contentlock

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

func roleOut(path, data string) Output { return Output{Path: path, Mode: 0o644, Data: []byte(data)} }

func lockWithRoles(t *testing.T, rendered map[string][]Output) *lockfile.File {
	t.Helper()
	snap := &Snapshot{}
	require.NoError(t, computeRoleOutputs(snap, rendered, nil))
	f := &lockfile.File{Version: lockfile.Version, Output: snap.Outputs}
	f.Tree = TreeOf(f)
	return f
}

func TestCompareRoles(t *testing.T) {
	base := map[string][]Output{
		"dev": {roleOut("CLAUDE.md", "a"), roleOut(".claude/settings.json", "{}")},
		"ops": {roleOut("CLAUDE.md", "b")},
	}
	tests := []struct {
		name     string
		rendered map[string][]Output
		only     []string
		want     []string // "<change> <role>"
	}{
		{name: "same bytes, no change", rendered: base},
		{name: "one file changed", rendered: map[string][]Output{
			"dev": {roleOut("CLAUDE.md", "a"), roleOut(".claude/settings.json", `{"x":1}`)}, "ops": base["ops"],
		}, want: []string{"changed dev"}},
		{name: "a file added", rendered: map[string][]Output{
			"dev": append(append([]Output{}, base["dev"]...), roleOut("AGENTS.md", "c")), "ops": base["ops"],
		}, want: []string{"changed dev"}},
		{name: "a role no longer rendered", rendered: map[string][]Output{"dev": base["dev"]}, want: []string{"removed ops"}},
		{name: "a role renamed", rendered: map[string][]Output{"dev": base["dev"], "qa": base["ops"]},
			want: []string{"added qa", "removed ops"}},
		{name: "only limits the comparison", rendered: map[string][]Output{"dev": nil}, only: []string{"dev"}, want: []string{"changed dev"}},
		{name: "only ignores other pins", rendered: map[string][]Output{"dev": base["dev"]}, only: []string{"dev"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			lock := lockWithRoles(t, base)

			// Act
			changes, err := CompareRoles(lock, tt.rendered, tt.only)

			// Assert
			require.NoError(t, err)
			var got []string
			for _, c := range changes {
				assert.Equal(t, ScopeOutput, c.Scope)
				assert.Equal(t, KindRoleOutput, c.Kind)
				got = append(got, c.Change+" "+c.ID)
			}
			assert.ElementsMatch(t, tt.want, got)
		})
	}
}

func TestRoleOutputsDoNotTouchDefaultOutputs(t *testing.T) {
	// Arrange
	snap := &Snapshot{}
	require.NoError(t, computeRoleOutputs(snap, map[string][]Output{"dev": {roleOut("CLAUDE.md", "a")}}, nil))
	withRole := &lockfile.File{Output: append([]lockfile.OutputPin{{Path: "CLAUDE.md", Digest: "sha256:1"}}, snap.Outputs...)}
	plain := &lockfile.File{Output: []lockfile.OutputPin{{Path: "CLAUDE.md", Digest: "sha256:1"}}}

	// Act / Assert
	assert.Len(t, withRole.DefaultOutputs(), 1)
	assert.Len(t, withRole.RoleOutputs(), 1)
	assert.NotEqual(t, TreeOf(plain), TreeOf(withRole), "the tree digest covers the role pin")
	d := &Diff{Changes: []Change{}}
	d.compareOutputs(withRole.DefaultOutputs(), defaultPins(append(plain.Output, snap.Outputs...)))
	assert.Empty(t, d.Changes, "a role pin is never read as a default output")
}

func TestRoleFilesOnlyInDiffMode(t *testing.T) {
	// Arrange
	lock := lockWithRoles(t, map[string][]Output{"dev": {roleOut("CLAUDE.md", "a")}})
	snap := &Snapshot{Options: Options{CheckRoles: true}}
	require.NoError(t, computeRoleOutputs(snap, map[string][]Output{"dev": {roleOut("CLAUDE.md", "b")}}, nil))

	// Act
	plain := &Diff{Changes: []Change{}}
	plain.compareRoleOutputs(lock, snap)
	snap.Options.RoleFiles = true
	verbose := &Diff{Changes: []Change{}}
	verbose.compareRoleOutputs(lock, snap)

	// Assert
	require.Len(t, plain.Changes, 1)
	assert.Empty(t, plain.Changes[0].Files)
	require.Len(t, verbose.Changes, 1)
	assert.Contains(t, verbose.Changes[0].Files, "CLAUDE.md")
}

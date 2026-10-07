package skillsource

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func manySkills(t *testing.T, n int) string {
	t.Helper()
	root := t.TempDir()
	for i := range n {
		name := "s" + strings.Repeat("x", i%3) + string(rune('a'+i%26)) + string(rune('a'+i/26))
		write(t, root, name+"/SKILL.md", skillMD(name, "Skill "+name))
	}
	return root
}

func TestDiscover_TooManySkillsIsAClearError(t *testing.T) {
	root := manySkills(t, 5)
	_, err := Discover(t.Context(), Spec{Name: "big", URL: root, MaxSkills: 3}, root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max_skills")
	assert.Contains(t, err.Error(), "5")

	skills, err := Discover(t.Context(), Spec{Name: "big", URL: root, MaxSkills: 5}, root)
	require.NoError(t, err)
	assert.Len(t, skills, 5)
	// include narrows what counts
	skills, err = Discover(t.Context(), Spec{Name: "big", URL: root, MaxSkills: 1, Include: []string{"sa*"}}, root)
	require.NoError(t, err)
	assert.Len(t, skills, 1)
}

func TestDiscover_TotalBytesOfASourceIsBounded(t *testing.T) {
	root := t.TempDir()
	for _, n := range []string{"a", "b", "c"} {
		write(t, root, n+"/SKILL.md", skillMD(n, "Skill "+n))
		write(t, root, n+"/references/data.md", strings.Repeat("x", 1000))
	}
	_, err := Discover(t.Context(), Spec{Name: "heavy", URL: root, MaxBytes: 2500}, root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max_bytes")
}

func TestDiscover_OnlyTheOversizeFileIsDropped(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a/SKILL.md", skillMD("a", "Skill a"))
	write(t, root, "a/references/ok.md", "fine")
	require.NoError(t, os.WriteFile(root+"/a/references/huge.bin", make([]byte, maxFileBytes+1), 0o644))
	skills, err := Discover(t.Context(), Spec{Name: "s", URL: root}, root)
	require.NoError(t, err)
	require.Len(t, skills, 1)
	var paths []string
	for _, f := range skills[0].Files {
		paths = append(paths, f.Path)
	}
	assert.Equal(t, []string{"SKILL.md", "references/ok.md"}, paths)
}

func TestGit_ACallThatOutlivesItsTimeoutFails(t *testing.T) {
	f := newFixture(t)
	old := gitTimeout
	gitTimeout = time.Nanosecond
	t.Cleanup(func() { gitTimeout = old })
	_, err := Resolve(context.Background(), Spec{Name: "t", URL: "git+" + f.url, Ref: "v1.0.0"}, Options{CacheDir: t.TempDir()})
	require.Error(t, err)
}

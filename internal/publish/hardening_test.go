package publish

import (
	"bytes"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/runner"
)

func problemPaths(res VerifyResult) []string {
	var out []string
	for _, p := range res.Problems {
		out = append(out, p.Path+": "+p.Message)
	}
	return out
}

func TestVerify_FlagsUnlistedAndDuplicateEntries(t *testing.T) {
	tests := []struct {
		name   string
		tamper func(t *testing.T, dir string)
		want   string
	}{
		{"unlisted file", func(t *testing.T, dir string) {
			require.NoError(t, os.WriteFile(filepath.Join(dir, "extra.sh"), []byte("x"), 0o600))
		}, "extra.sh: present but not listed"},
		{"unlisted nested file", func(t *testing.T, dir string) {
			require.NoError(t, os.MkdirAll(filepath.Join(dir, "emit"), 0o750))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "emit", "a.json"), []byte("x"), 0o600))
		}, "emit/a.json: present but not listed"},
		{"duplicate sums line", func(t *testing.T, dir string) {
			raw, err := os.ReadFile(filepath.Join(dir, SumsFile))
			require.NoError(t, err)
			first := strings.SplitN(string(raw), "\n", 2)[0]
			appendTo(t, filepath.Join(dir, SumsFile), first+"\n")
		}, "listed more than once"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, _ := writeDist(t)
			tt.tamper(t, dir)
			res, err := Verify(dir)
			require.NoError(t, err)
			require.False(t, res.OK())
			assert.Contains(t, strings.Join(problemPaths(res), "\n"), tt.want)
		})
	}
}

func TestVerify_ChecksLockTreeAgainstTheManifest(t *testing.T) {
	// Arrange: the lock copy and every digest agree with each other, but the
	// manifest claims another tree.
	in := sampleInput()
	in.LockTree = Digest([]byte("another tree"))
	d, err := Build(in)
	require.NoError(t, err)
	dir := filepath.Join(t.TempDir(), "dist")
	require.NoError(t, d.Write(dir))

	// Act
	res, err := Verify(dir)

	// Assert
	require.NoError(t, err)
	require.False(t, res.OK())
	assert.Contains(t, strings.Join(problemPaths(res), "\n"), "lock tree")
}

func TestReadArchive_CapsTheTotalSize(t *testing.T) {
	// Arrange
	old := maxVerifyBytes
	maxVerifyBytes = 10
	t.Cleanup(func() { maxVerifyBytes = old })
	archive, err := BuildArchive([]File{{Path: "a", Data: bytes.Repeat([]byte("x"), 6)}, {Path: "b", Data: bytes.Repeat([]byte("x"), 6)}}, 0)
	require.NoError(t, err)

	// Act
	_, err = readArchive(archive)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds")
}

func TestReadArchive_CapsAGzipBomb(t *testing.T) {
	// Arrange: one entry larger than the cap.
	old := maxVerifyBytes
	maxVerifyBytes = 1024
	t.Cleanup(func() { maxVerifyBytes = old })
	archive, err := BuildArchive([]File{{Path: "a", Data: bytes.Repeat([]byte("x"), 1<<16)}}, 0)
	require.NoError(t, err)
	zr, err := gzip.NewReader(bytes.NewReader(archive))
	require.NoError(t, err)
	require.NoError(t, zr.Close())

	// Act
	_, err = readArchive(archive)

	// Assert
	require.Error(t, err)
}

// fakeRunner records every spec and answers from a table keyed by git subcommand.
type fakeRunner struct {
	specs []runner.Spec
	out   map[string]string
}

func (f *fakeRunner) Run(_ context.Context, spec runner.Spec) runner.Result {
	f.specs = append(f.specs, spec)
	for _, a := range spec.Argv[1:] {
		if v, ok := f.out[a]; ok {
			return runner.Result{Status: runner.StatusOK, Stdout: []byte(v)}
		}
	}
	return runner.Result{Status: runner.StatusError, ExitCode: 1}
}

func TestReadSource_UsesShortTimeoutsAndListsEveryUntrackedFile(t *testing.T) {
	// Arrange
	f := &fakeRunner{out: map[string]string{"rev-parse": "abc\n", "log": "1700000000\n", "status": "", "remote": "https://x/y\n"}}

	// Act
	info := ReadSource(context.Background(), f, "/proj", "dist")

	// Assert
	require.Len(t, f.specs, 4)
	for _, s := range f.specs {
		assert.LessOrEqual(t, s.Timeout, gitQueryTimeout, strings.Join(s.Argv, " "))
		assert.NotZero(t, s.Timeout)
	}
	var status []string
	for _, s := range f.specs {
		for _, a := range s.Argv {
			if a == "status" {
				status = s.Argv
			}
		}
	}
	assert.Contains(t, status, "--untracked-files=all")
	assert.False(t, info.Source.Dirty)
	assert.Equal(t, "abc", info.Source.Commit)
}

func TestDistWrite_RefusesSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on windows")
	}
	tests := []struct {
		name  string
		plant func(t *testing.T, dir, outside string)
	}{
		{"artifact leaf is a symlink", func(t *testing.T, dir, outside string) {
			require.NoError(t, os.Symlink(filepath.Join(outside, "victim"), filepath.Join(dir, LockFile)))
		}},
		{"emit is a symlinked directory", func(t *testing.T, dir, outside string) {
			require.NoError(t, os.Symlink(outside, filepath.Join(dir, EmitDir)))
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange: an earlier publish whose output now contains a planted link.
			in := sampleInput()
			in.Templates = []Template{{Name: "x.json.tmpl", Body: "{}"}}
			d, err := Build(in)
			require.NoError(t, err)
			dir := filepath.Join(t.TempDir(), "dist")
			require.NoError(t, d.Write(dir))
			outside := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(outside, "victim"), []byte("precious"), 0o600))
			require.NoError(t, os.Remove(filepath.Join(dir, LockFile)))
			require.NoError(t, os.RemoveAll(filepath.Join(dir, EmitDir)))
			tt.plant(t, dir, outside)

			// Act
			err = d.Write(dir)

			// Assert
			var pe *Error
			require.ErrorAs(t, err, &pe)
			assert.Equal(t, CodeBundleUnsafe, pe.Code)
			data, rerr := os.ReadFile(filepath.Join(outside, "victim"))
			require.NoError(t, rerr)
			assert.Equal(t, "precious", string(data))
			entries, rerr := os.ReadDir(outside)
			require.NoError(t, rerr)
			assert.Len(t, entries, 1, "nothing was written through the link")
		})
	}
}

func TestDistWrite_RemovalDoesNotFollowSymlinkedDirectories(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on windows")
	}
	// Arrange: an earlier plan lists emit/x.json and emit has become a link.
	in := sampleInput()
	in.Templates = []Template{{Name: "x.json.tmpl", Body: "{}"}}
	d, err := Build(in)
	require.NoError(t, err)
	dir := filepath.Join(t.TempDir(), "dist")
	require.NoError(t, d.Write(dir))
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "x.json"), []byte("precious"), 0o600))
	require.NoError(t, os.RemoveAll(filepath.Join(dir, EmitDir)))
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, EmitDir)))
	next, err := Build(sampleInput()) // no template: emit/x.json is only in the old plan

	// Act
	err = next.Write(dir)

	// Assert
	require.Error(t, err)
	assert.FileExists(t, filepath.Join(outside, "x.json"))
}

func TestDistWrite_LeavesAReusableDirectoryWhenAnInstallFails(t *testing.T) {
	// Arrange: an earlier publish, then a new build whose emit target is blocked
	// by a directory, so installing fails midway.
	dir, _ := writeDist(t)
	next := sampleInput()
	next.Version = "1.5.0"
	next.Templates = []Template{{Name: "x.json.tmpl", Body: "{}"}}
	d, err := Build(next)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, EmitDir, "x.json", "sub"), 0o750))

	// Act
	err = d.Write(dir)

	// Assert: the old plan is intact so the directory can be published into again.
	require.Error(t, err)
	assert.FileExists(t, filepath.Join(dir, PlanFile))
	entries, rerr := os.ReadDir(filepath.Dir(dir))
	require.NoError(t, rerr)
	for _, e := range entries {
		assert.Equal(t, "dist", e.Name(), "no staging directory is left behind")
	}
	require.NoError(t, os.RemoveAll(filepath.Join(dir, EmitDir)))
	again, err := Build(sampleInput())
	require.NoError(t, err)
	require.NoError(t, again.Write(dir))
}

func TestDistWrite_FreshDirectoryIsInstalledWholeOrNotAtAll(t *testing.T) {
	// Arrange
	d, err := Build(sampleInput())
	require.NoError(t, err)
	parent := t.TempDir()
	dir := filepath.Join(parent, "nested", "dist")

	// Act
	require.NoError(t, d.Write(dir))

	// Assert
	for _, p := range d.Paths() {
		assert.FileExists(t, filepath.Join(dir, filepath.FromSlash(p)))
	}
	entries, err := os.ReadDir(filepath.Join(parent, "nested"))
	require.NoError(t, err)
	assert.Len(t, entries, 1)
	res, err := Verify(dir)
	require.NoError(t, err)
	assert.True(t, res.OK(), "%v", res.Problems)
}

func githubInput() Input {
	in := sampleInput()
	in.Target, in.Tag, in.Repo = TargetGitHubRelease, "v1.4.0", "acme/skills"
	return in
}

func TestVerify_ChecksThePlan(t *testing.T) {
	tests := []struct {
		name   string
		tamper func(t *testing.T, dir string)
		want   string
	}{
		{"edited size", func(t *testing.T, dir string) {
			replaceIn(t, filepath.Join(dir, PlanFile), `"size": 12`, `"size": 999999`)
		}, "plan records 999999 bytes"},
		{"extra command", func(t *testing.T, dir string) {
			replaceIn(t, filepath.Join(dir, PlanFile), `"commands": [`, `"commands": [{"argv": ["gh", "repo", "delete", "acme/skills"], "cwd": "."},`)
		}, "plan commands differ"},
		{"changed repo", func(t *testing.T, dir string) {
			replaceIn(t, filepath.Join(dir, PlanFile), `"repo": "acme/skills"`, `"repo": "evil/skills"`)
		}, "plan commands differ"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, err := Build(githubInput())
			require.NoError(t, err)
			dir := filepath.Join(t.TempDir(), "dist")
			require.NoError(t, d.Write(dir))
			if tt.name == "edited size" {
				// the lock copy is the smallest known size; target it by its recorded value
				var size int
				for _, a := range d.Plan.Artifacts {
					if a.Path == LockFile {
						size = a.Size
					}
				}
				tt.tamper = func(t *testing.T, dir string) {
					replaceIn(t, filepath.Join(dir, PlanFile), `"size": `+strconv.Itoa(size), `"size": 999999`)
				}
			}
			tt.tamper(t, dir)
			res, err := Verify(dir)
			require.NoError(t, err)
			require.False(t, res.OK())
			assert.Contains(t, strings.Join(problemPaths(res), "\n"), tt.want)
		})
	}
}

func TestVerify_AcceptsAGitHubPlan(t *testing.T) {
	d, err := Build(githubInput())
	require.NoError(t, err)
	dir := filepath.Join(t.TempDir(), "dist")
	require.NoError(t, d.Write(dir))
	res, err := Verify(dir)
	require.NoError(t, err)
	assert.True(t, res.OK(), "%v", res.Problems)
}

func TestValidateTarget_RejectsOptionLookingRepos(t *testing.T) {
	for _, repo := range []string{"-acme/x", "acme/-x", "-h.com/acme/x", ".acme/x"} {
		assert.Error(t, ValidateTarget("v1", repo), repo)
	}
	assert.NoError(t, ValidateTarget("v1", "acme/.github"))
	assert.NoError(t, ValidateTarget("v1", "ghe.example.com/acme/x"))
}

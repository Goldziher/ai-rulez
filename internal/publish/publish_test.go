package publish

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"

	"github.com/kaptinlin/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/runner"
)

func sampleInput() Input {
	return Input{
		Name: "acme", Version: "1.4.0", AIRulezVersion: "5.0.0", Runtimes: []string{"codex", "claude"},
		Files: []File{
			{Path: "skills/b/SKILL.md", Data: []byte("b")},
			{Path: ".claude-plugin/plugin.json", Data: []byte("{}")},
			{Path: "hooks/run.sh", Data: []byte("#!/bin/sh\n"), Executable: true},
		},
		Lock: []byte("version = 2\ntree = \"" + Digest([]byte("tree")) + "\"\n"), LockVersion: 2, LockTree: Digest([]byte("tree")),
		Source: Source{Repo: "https://github.com/acme/skills", Commit: "0f3e"}, Mtime: 1700000000,
	}
}

func TestValidPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"skills/a/SKILL.md", true},
		{".claude-plugin/plugin.json", true},
		{"", false},
		{"/etc/passwd", false},
		{"../x", false},
		{"a/../b", false},
		{"a//b", false},
		{"./a", false},
		{"a\\b", false},
		{"C:/x", false},
		{"a\x00b", false},
		{".", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			assert.Equal(t, tt.want, ValidPath(tt.path))
		})
	}
}

func TestBuildArchive_IsDeterministicAndNormalised(t *testing.T) {
	// Arrange
	in := sampleInput()
	reversed := append([]File(nil), in.Files...)
	reversed[0], reversed[2] = reversed[2], reversed[0]

	// Act
	first, err := BuildArchive(in.Files, in.Mtime)
	require.NoError(t, err)
	second, err := BuildArchive(reversed, in.Mtime)
	require.NoError(t, err)

	// Assert
	assert.Equal(t, first, second, "input order must not matter")
	assert.Equal(t, []byte{0x1f, 0x8b}, first[:2])
	assert.Equal(t, []byte{0, 0, 0, 0}, first[4:8], "gzip header carries no mtime")
	assert.Equal(t, byte(0), first[3], "gzip header has no name or comment flag")

	zr, err := gzip.NewReader(bytes.NewReader(first))
	require.NoError(t, err)
	tr := tar.NewReader(zr)
	var names []string
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		names = append(names, hdr.Name)
		assert.Equal(t, int64(1700000000), hdr.ModTime.Unix())
		assert.Zero(t, hdr.Uid)
		assert.Zero(t, hdr.Gid)
		assert.Empty(t, hdr.Uname)
		assert.Empty(t, hdr.Gname)
		assert.Empty(t, hdr.PAXRecords)
		if hdr.Name == "hooks/run.sh" {
			assert.Equal(t, int64(0o755), hdr.Mode)
		} else {
			assert.Equal(t, int64(0o644), hdr.Mode)
		}
	}
	assert.Equal(t, []string{".claude-plugin/plugin.json", "hooks/run.sh", "skills/b/SKILL.md"}, names)
}

func TestBuildArchive_RejectsUnsafeInput(t *testing.T) {
	tests := []struct {
		name  string
		files []File
	}{
		{"parent escape", []File{{Path: "../x", Data: []byte("x")}}},
		{"absolute", []File{{Path: "/x", Data: []byte("x")}}},
		{"duplicate", []File{{Path: "a", Data: []byte("1")}, {Path: "a", Data: []byte("2")}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := BuildArchive(tt.files, 0)
			var pe *Error
			require.ErrorAs(t, err, &pe)
			assert.Equal(t, CodeBundleUnsafe, pe.Code)
		})
	}
}

func TestBuild_IsReproducible(t *testing.T) {
	// Act
	a, err := Build(sampleInput())
	require.NoError(t, err)
	b, err := Build(sampleInput())
	require.NoError(t, err)

	// Assert
	assert.Equal(t, a.Paths(), b.Paths())
	for _, p := range a.Paths() {
		assert.Equal(t, a.Files[p], b.Files[p], p)
	}
	assert.Equal(t, []string{"RELEASE_NOTES.md", "SHA256SUMS", "acme-1.4.0.manifest.json", "acme-1.4.0.tar.gz", "ai-rulez.lock", "publish-plan.json"}, a.Paths())
	assert.Equal(t, []string{"claude", "codex"}, a.Manifest.Runtimes)
	assert.Equal(t, Digest(a.Files["acme-1.4.0.tar.gz"]), a.Manifest.Bundle.Digest)
	assert.Equal(t, Digest(sampleInput().Lock), a.Manifest.Lock.FileDigest)
}

func TestBuild_ValidatesNameVersionAndTarget(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Input)
		wantErr string
	}{
		{"no version", func(i *Input) { i.Version = "" }, "version is not set"},
		{"unsafe name", func(i *Input) { i.Name = "../evil" }, "cannot name release files"},
		{"leading dash tag", func(i *Input) { i.Target, i.Tag, i.Repo = TargetGitHubRelease, "-rf", "a/b" }, "invalid release tag"},
		{"bad repo", func(i *Input) { i.Target, i.Tag, i.Repo = TargetGitHubRelease, "v1", "not a repo" }, "OWNER/REPO"},
		{"unknown target", func(i *Input) { i.Target = "npm" }, "unknown target"},
		{"no lock", func(i *Input) { i.Lock = nil }, "ai-rulez.lock is missing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := sampleInput()
			tt.mutate(&input)
			_, err := Build(input)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestBuild_PlanHasFixedArgv(t *testing.T) {
	// Arrange
	input := sampleInput()
	input.Target, input.Tag, input.Repo = TargetGitHubRelease, "v1.4.0", "acme/skills"

	// Act
	d, err := Build(input)
	require.NoError(t, err)

	// Assert
	require.Len(t, d.Plan.Commands, 1)
	assert.Equal(t, []string{
		"gh", "release", "create", "v1.4.0", "--repo", "acme/skills", "--title", "acme 1.4.0",
		"--notes-file", "RELEASE_NOTES.md", "--verify-tag",
		"acme-1.4.0.tar.gz", "acme-1.4.0.manifest.json", "ai-rulez.lock", "SHA256SUMS",
	}, d.Plan.Commands[0].Argv)
	var plan Plan
	require.NoError(t, json.Unmarshal(d.Files[PlanFile], &plan))
	assert.Equal(t, d.Plan.Upload, plan.Upload)
}

func TestBuild_TemplatesRenderIntoEmit(t *testing.T) {
	// Arrange
	input := sampleInput()
	input.Templates = []Template{{Name: "index.json.tmpl", Body: `{"name":{{json .Name}},"digest":"{{.BundleDigest}}","n":{{len .Files}}}`}}

	// Act
	d, err := Build(input)
	require.NoError(t, err)

	// Assert
	body := string(d.Files["emit/index.json"])
	assert.Contains(t, body, `"name":"acme"`)
	assert.Contains(t, body, d.Manifest.Bundle.Digest)
	assert.Contains(t, body, `"n":3`)
	assert.Contains(t, string(d.Files[SumsFile]), "emit/index.json")
}

func TestBuild_TemplateErrors(t *testing.T) {
	tests := []struct {
		name string
		tpls []Template
	}{
		{"unknown field", []Template{{Name: "a.tmpl", Body: "{{.Nope}}"}}},
		{"syntax error", []Template{{Name: "a.tmpl", Body: "{{"}}},
		{"duplicate output", []Template{{Name: "a.tmpl", Body: "x"}, {Name: "a", Body: "y"}}},
		{"no name", []Template{{Name: ".tmpl", Body: "x"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := sampleInput()
			input.Templates = tt.tpls
			_, err := Build(input)
			assert.Error(t, err)
		})
	}
}

func writeDist(t *testing.T) (string, *Dist) {
	t.Helper()
	d, err := Build(sampleInput())
	require.NoError(t, err)
	dir := filepath.Join(t.TempDir(), "dist")
	require.NoError(t, d.Write(dir))
	return dir, d
}

func TestVerify_AcceptsAFreshDist(t *testing.T) {
	// Arrange
	dir, _ := writeDist(t)

	// Act
	res, err := Verify(dir)

	// Assert
	require.NoError(t, err)
	assert.True(t, res.OK(), "%v", res.Problems)
	assert.Equal(t, "acme", res.Name)
	assert.Equal(t, 4, res.Files)
}

func TestVerify_ReportsTampering(t *testing.T) {
	tests := []struct {
		name   string
		tamper func(t *testing.T, dir string)
		want   string
	}{
		{"flipped archive byte", func(t *testing.T, dir string) { appendTo(t, filepath.Join(dir, "acme-1.4.0.tar.gz"), "x") }, "acme-1.4.0.tar.gz"},
		{"edited lock", func(t *testing.T, dir string) { appendTo(t, filepath.Join(dir, LockFile), "# x\n") }, LockFile},
		{"edited manifest", func(t *testing.T, dir string) { appendTo(t, filepath.Join(dir, "acme-1.4.0.manifest.json"), " ") }, "acme-1.4.0.manifest.json"},
		{"deleted artifact", func(t *testing.T, dir string) { require.NoError(t, os.Remove(filepath.Join(dir, NotesFile))) }, NotesFile},
		{"edited plan", func(t *testing.T, dir string) { replaceIn(t, filepath.Join(dir, PlanFile), "1.4.0", "9.9.9") }, "acme-9.9.9.tar.gz"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, _ := writeDist(t)
			tt.tamper(t, dir)
			res, err := Verify(dir)
			require.NoError(t, err)
			require.False(t, res.OK())
			var paths []string
			for _, p := range res.Problems {
				paths = append(paths, p.Path)
			}
			assert.Contains(t, paths, tt.want)
		})
	}
}

func TestVerify_ChecksManifestAgainstArchive(t *testing.T) {
	// Arrange: a manifest that lies about a file, with every checksum recomputed so only the cross-check can catch it.
	dir, d := writeDist(t)
	m := d.Manifest
	m.Files[0].Digest = Digest([]byte("other"))
	manifest, err := m.Marshal()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "acme-1.4.0.manifest.json"), manifest, 0o600))
	d.Files["acme-1.4.0.manifest.json"] = manifest
	var sums []SumEntry
	for p, data := range d.Files {
		if p != SumsFile && p != PlanFile {
			sums = append(sums, SumEntry{Path: p, Digest: Digest(data)})
		}
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, SumsFile), FormatSums(sums), 0o600))
	require.NoError(t, os.Remove(filepath.Join(dir, PlanFile)))

	// Act
	res, err := Verify(dir)

	// Assert
	require.NoError(t, err)
	require.False(t, res.OK())
	assert.Contains(t, res.Problems[0].Message, "archive holds")
}

func TestVerify_MissingSumsIsAnError(t *testing.T) {
	_, err := Verify(t.TempDir())
	var pe *Error
	require.ErrorAs(t, err, &pe)
	assert.Equal(t, CodeVerify, pe.Code)
	assert.Equal(t, ExitFailed, pe.Exit)
}

func TestVerify_RejectsNonDeterministicArchive(t *testing.T) {
	tests := []struct {
		name  string
		build func(*tar.Writer)
	}{
		{"owner", func(tw *tar.Writer) { writeEntry(t, tw, &tar.Header{Name: "a", Mode: 0o644, Uid: 1000}, "x") }},
		{"mode", func(tw *tar.Writer) { writeEntry(t, tw, &tar.Header{Name: "a", Mode: 0o600}, "x") }},
		{"order", func(tw *tar.Writer) {
			writeEntry(t, tw, &tar.Header{Name: "b", Mode: 0o644}, "x")
			writeEntry(t, tw, &tar.Header{Name: "a", Mode: 0o644}, "x")
		}},
		{"symlink", func(tw *tar.Writer) {
			require.NoError(t, tw.WriteHeader(&tar.Header{Name: "a", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"}))
		}},
		{"escape", func(tw *tar.Writer) { writeEntry(t, tw, &tar.Header{Name: "../a", Mode: 0o644}, "x") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			zw := gzip.NewWriter(&buf)
			tw := tar.NewWriter(zw)
			tt.build(tw)
			require.NoError(t, tw.Close())
			require.NoError(t, zw.Close())
			_, err := readArchive(buf.Bytes())
			assert.Error(t, err)
		})
	}
}

func writeEntry(t *testing.T, tw *tar.Writer, hdr *tar.Header, body string) {
	t.Helper()
	hdr.Typeflag, hdr.Size = tar.TypeReg, int64(len(body))
	require.NoError(t, tw.WriteHeader(hdr))
	_, err := tw.Write([]byte(body))
	require.NoError(t, err)
}

func TestDistWrite_ReplacesEarlierOutputOnly(t *testing.T) {
	// Arrange
	dir, _ := writeDist(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "stale-0.1.tar.gz"), []byte("old"), 0o600))
	newer := sampleInput()
	newer.Version = "1.5.0"
	d, err := Build(newer)
	require.NoError(t, err)

	// Act
	err = d.Write(dir)

	// Assert: the stranger file stays, the old artifacts are gone, the new ones are in.
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(dir, "stale-0.1.tar.gz"))
	assert.NoFileExists(t, filepath.Join(dir, "acme-1.4.0.tar.gz"))
	assert.FileExists(t, filepath.Join(dir, "acme-1.5.0.tar.gz"))
}

func TestDistWrite_RefusesAForeignDirectory(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "precious.txt"), []byte("x"), 0o600))
	d, err := Build(sampleInput())
	require.NoError(t, err)

	// Act
	err = d.Write(dir)

	// Assert
	require.Error(t, err)
	assert.FileExists(t, filepath.Join(dir, "precious.txt"))
}

func TestCheckTree_RejectsSymlinks(t *testing.T) {
	// Arrange
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "real"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(root, "real", "f"), []byte("x"), 0o600))
	testutil.SymlinkOrSkip(t, filepath.Join(root, "real"), filepath.Join(root, "link"))
	testutil.SymlinkOrSkip(t, filepath.Join(root, "real", "f"), filepath.Join(root, "flink"))

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"plain file", "real/f", false},
		{"through a linked directory", "link/f", true},
		{"a linked file", "flink", true},
		{"missing", "nope", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckTree(root, []string{tt.path})
			assert.Equal(t, tt.wantErr, err != nil, "%v", err)
		})
	}
}

func TestRepoFromURL(t *testing.T) {
	tests := map[string]string{
		"https://github.com/acme/skills.git":     "acme/skills",
		"https://github.com/acme/skills":         "acme/skills",
		"git@github.com:acme/skills.git":         "acme/skills",
		"ssh://git@github.com/acme/skills.git":   "acme/skills",
		"https://ghe.example.com/acme/skills":    "ghe.example.com/acme/skills",
		"acme/skills":                            "acme/skills",
		"https://github.com/acme":                "",
		"https://github.com/acme/skills/extra/x": "",
	}
	for in, want := range tests {
		t.Run(in, func(t *testing.T) {
			assert.Equal(t, want, RepoFromURL(in))
		})
	}
}

func TestStripCredentials(t *testing.T) {
	assert.Equal(t, "https://github.com/a/b", StripCredentials("https://user:tok@github.com/a/b?x=1#f"))
	assert.Equal(t, "git@github.com:a/b.git", StripCredentials("git@github.com:a/b.git"))
}

func TestParseSums_RejectsMalformed(t *testing.T) {
	good := strings.Repeat("a", 64)
	tests := []struct {
		name string
		in   string
		ok   bool
	}{
		{"ok", good + "  file\n", true},
		{"one space", good + " file\n", false},
		{"short digest", "abc  file\n", false},
		{"upper digest", strings.ToUpper(good) + "  file\n", false},
		{"escaping path", good + "  ../file\n", false},
		{"blank line", good + "  a\n\n" + good + "  b\n", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseSums([]byte(tt.in))
			assert.Equal(t, tt.ok, err == nil)
		})
	}
}

func execPlan() Plan {
	input := sampleInput()
	input.Target, input.Tag, input.Repo = TargetGitHubRelease, "v1.4.0", "acme/skills"
	d, _ := Build(input) //nolint:errcheck // the input is valid
	return d.Plan
}

func TestExecute(t *testing.T) {
	notFound := runner.Result{Status: runner.StatusExit, ExitCode: 1, Stderr: []byte("release not found")}
	tests := []struct {
		name      string
		force     bool
		view      runner.Result
		create    runner.Result
		wantCalls []string
		wantErr   string
		wantOut   string
	}{
		{"creates a new release", false, notFound, runner.Result{Status: runner.StatusOK, Stdout: []byte("https://example/r\n")}, []string{"view", "create"}, "", "https://example/r"},
		{"refuses an existing release", false, runner.Result{Status: runner.StatusOK}, runner.Result{}, []string{"view"}, "already exists", ""},
		{"force uploads with clobber", true, runner.Result{Status: runner.StatusOK}, runner.Result{Status: runner.StatusOK, Stdout: []byte("done")}, []string{"view", "upload"}, "", "done"},
		{"gh missing", false, runner.Result{Status: runner.StatusUnavailable, Err: os.ErrNotExist}, runner.Result{}, []string{"view"}, "gh was not found", ""},
		{"auth failure is not 'not found'", false, runner.Result{Status: runner.StatusExit, ExitCode: 4, Stderr: []byte("authentication required")}, runner.Result{}, []string{"view"}, "authentication required", ""},
		{"create fails", false, notFound, runner.Result{Status: runner.StatusExit, ExitCode: 1, Stderr: []byte("tag v1.4.0 not pushed")}, []string{"view", "create"}, "not pushed", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			fake := &runner.Fake{Handle: func(spec runner.Spec) runner.Result {
				if spec.Argv[2] == "view" {
					return tt.view
				}
				return tt.create
			}}

			// Act
			out, err := Execute(context.Background(), fake, execPlan(), ExecuteOptions{Dir: "/d", Env: []string{"A=b"}, Force: tt.force})

			// Assert
			var verbs []string
			for _, c := range fake.Calls() {
				assert.Equal(t, "gh", c.Argv[0])
				assert.Equal(t, "/d", c.Dir)
				assert.Equal(t, []string{"A=b"}, c.Env)
				assert.False(t, c.InheritEnv)
				verbs = append(verbs, c.Argv[2])
			}
			assert.Equal(t, tt.wantCalls, verbs)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				var pe *Error
				require.ErrorAs(t, err, &pe)
				assert.Equal(t, CodeTarget, pe.Code)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantOut, out)
		})
	}
}

func TestExecute_CreateUsesThePlannedArgv(t *testing.T) {
	// Arrange
	plan := execPlan()
	fake := &runner.Fake{Handle: func(spec runner.Spec) runner.Result {
		if spec.Argv[2] == "view" {
			return runner.Result{Status: runner.StatusExit, ExitCode: 1, Stderr: []byte("release not found")}
		}
		return runner.Result{Status: runner.StatusOK}
	}}

	// Act
	_, err := Execute(context.Background(), fake, plan, ExecuteOptions{})

	// Assert
	require.NoError(t, err)
	calls := fake.Calls()
	require.Len(t, calls, 2)
	assert.Equal(t, plan.Commands[0].Argv, calls[1].Argv)
}

func TestExecute_NeedsAGitHubPlan(t *testing.T) {
	_, err := Execute(context.Background(), &runner.Fake{}, Plan{}, ExecuteOptions{})
	assert.Error(t, err)
}

func appendTo(t *testing.T, path, s string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	require.NoError(t, err)
	_, err = f.WriteString(s)
	require.NoError(t, err)
	require.NoError(t, f.Close())
}

func replaceIn(t *testing.T, path, old, repl string) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, []byte(strings.ReplaceAll(string(data), old, repl)), 0o600))
}

func TestSchemas_AcceptTheDocuments(t *testing.T) {
	// Arrange
	input := sampleInput()
	input.Target, input.Tag, input.Repo = TargetGitHubRelease, "v1.4.0", "acme/skills"
	input.Templates = []Template{{Name: "x.tmpl", Body: "x"}}
	d, err := Build(input)
	require.NoError(t, err)
	docs := map[string][]byte{
		"publish-manifest.schema.json": d.Files["acme-1.4.0.manifest.json"],
		"publish-plan.schema.json":     d.Files[PlanFile],
	}

	for name, doc := range docs {
		t.Run(name, func(t *testing.T) {
			// Act
			raw, err := os.ReadFile(filepath.Join("..", "..", "schema", name))
			require.NoError(t, err)
			schema, err := jsonschema.NewCompiler().Compile(raw)
			require.NoError(t, err)
			result := schema.Validate(doc)

			// Assert
			assert.True(t, result.IsValid(), "%v", result.Errors)
		})
	}
}

func TestSchemas_RejectAnUnknownField(t *testing.T) {
	// Arrange
	raw, err := os.ReadFile(filepath.Join("..", "..", "schema", "publish-manifest.schema.json"))
	require.NoError(t, err)
	schema, err := jsonschema.NewCompiler().Compile(raw)
	require.NoError(t, err)
	d, err := Build(sampleInput())
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(d.Files["acme-1.4.0.manifest.json"], &doc))
	doc["surprise"] = true
	mutated, err := json.Marshal(doc)
	require.NoError(t, err)

	// Act + Assert
	assert.False(t, schema.Validate(mutated).IsValid())
}

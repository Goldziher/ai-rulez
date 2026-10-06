package publish

import (
	"context"
	"crypto"
	"encoding/json"
	"io"
	"log"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/kaptinlin/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/publish/emit"
	"github.com/Goldziher/ai-rulez/v5/internal/publish/oci"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
	"github.com/Goldziher/ai-rulez/v5/internal/signing"
)

const commit40 = "0f3e0f3e0f3e0f3e0f3e0f3e0f3e0f3e0f3e0f3e"

func schemaValid(t *testing.T, schemaFile string, doc []byte) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "schema", schemaFile))
	require.NoError(t, err)
	schema, err := jsonschema.NewCompiler().Compile(raw)
	require.NoError(t, err)
	result := schema.Validate(doc)
	assert.True(t, result.IsValid(), "%s: %v", schemaFile, result.Errors)
}

func committedInput() Input {
	in := sampleInput()
	in.Source.Commit = commit40
	in.Files = append(in.Files, File{Path: "skills/deploy/SKILL.md", Data: []byte("---\nname: deploy\ndescription: Deploy it.\n---\nBody\n")})
	return in
}

const claudeIndex = `{
  "name": "acme-skills",
  "owner": {"name": "Acme"},
  "plugins": [
    {"name": "acme", "source": "./", "description": "Conventions", "version": "1.4.0", "keywords": ["x"]},
    {"name": "acme-extra", "source": "./plugins/extra", "description": "Extra"}
  ]
}`

func pinFor(index string) *Pin {
	return &Pin{Index: []byte(index), Repo: "acme/skills", Ref: "v1.4.0"}
}

// pinnedEntries decodes a pinned index into its plugin entries.
func pinnedEntries(t *testing.T, data []byte) []map[string]any {
	t.Helper()
	var doc struct {
		Plugins []map[string]any `json:"plugins"`
	}
	require.NoError(t, json.Unmarshal(data, &doc))
	return doc.Plugins
}

func TestPinIndex(t *testing.T) {
	tests := []struct {
		name   string
		pin    func(*Pin)
		want0  map[string]any
		want1  map[string]any
		errMsg string
	}{
		{"github root and subdirectory", func(p *Pin) {},
			map[string]any{"source": "github", "repo": "acme/skills", "ref": "v1.4.0", "sha": commit40},
			map[string]any{"source": "git-subdir", "url": "https://github.com/acme/skills.git", "path": "plugins/extra", "ref": "v1.4.0", "sha": commit40},
			""},
		{"project below the repository root", func(p *Pin) { p.RepoPath = "tools/skills" },
			map[string]any{"source": "git-subdir", "url": "https://github.com/acme/skills.git", "path": "tools/skills", "ref": "v1.4.0", "sha": commit40},
			map[string]any{"source": "git-subdir", "url": "https://github.com/acme/skills.git", "path": "tools/skills/plugins/extra", "ref": "v1.4.0", "sha": commit40},
			""},
		{"marketplace root below the project", func(p *Pin) { p.IndexRoot = "market" },
			map[string]any{"source": "git-subdir", "url": "https://github.com/acme/skills.git", "path": "market", "ref": "v1.4.0", "sha": commit40},
			map[string]any{"source": "git-subdir", "url": "https://github.com/acme/skills.git", "path": "market/plugins/extra", "ref": "v1.4.0", "sha": commit40},
			""},
		{"other host", func(p *Pin) { p.Repo = "git.example.com/acme/skills" },
			map[string]any{"source": "url", "url": "https://git.example.com/acme/skills.git", "ref": "v1.4.0", "sha": commit40},
			map[string]any{"source": "git-subdir", "url": "https://git.example.com/acme/skills.git", "path": "plugins/extra", "ref": "v1.4.0", "sha": commit40},
			""},
		{"channel ref", func(p *Pin) { p.Ref = "main" },
			map[string]any{"source": "github", "repo": "acme/skills", "ref": "main", "sha": commit40},
			map[string]any{"source": "git-subdir", "url": "https://github.com/acme/skills.git", "path": "plugins/extra", "ref": "main", "sha": commit40},
			""},
		{"bad ref", func(p *Pin) { p.Ref = "../x" }, nil, nil, "invalid git ref"},
		{"bad repo", func(p *Pin) { p.Repo = "nope" }, nil, nil, "OWNER/REPO"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := pinFor(claudeIndex)
			tt.pin(p)

			out, err := PinIndex(*p, commit40)

			if tt.errMsg != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				return
			}
			require.NoError(t, err)
			entries := pinnedEntries(t, out)
			require.Len(t, entries, 2)
			assert.Equal(t, tt.want0, entries[0]["source"])
			assert.Equal(t, tt.want1, entries[1]["source"])
			assert.Equal(t, "Conventions", entries[0]["description"], "other entry fields are kept")
			assert.Equal(t, []any{"x"}, entries[0]["keywords"])
			assert.Contains(t, string(out), `"owner"`)
		})
	}
}

func TestPinIndex_RefusesWhatCannotBePinned(t *testing.T) {
	tests := []struct {
		name, index, commit, want string
	}{
		{"no commit", claudeIndex, "", "no commit"},
		{"short commit", claudeIndex, "0f3e", "no commit"},
		{"already an object source", `{"name":"m","plugins":[{"name":"a","source":{"source":"github","repo":"x/y"}}]}`, commit40, "no relative source"},
		{"escaping source", `{"name":"m","plugins":[{"name":"a","source":"./../x"}]}`, commit40, "relative path inside"},
		{"absolute source", `{"name":"m","plugins":[{"name":"a","source":"/etc"}]}`, commit40, "relative path inside"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := PinIndex(*pinFor(tt.index), tt.commit)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestBuild_WritesAPinnedIndexPerChannel(t *testing.T) {
	tests := []struct {
		channel, path, ref string
	}{
		{"", "marketplace/.claude-plugin/marketplace.json", "v1.4.0"},
		{"stable", "marketplace/stable/.claude-plugin/marketplace.json", "v1.4.0"},
		{"canary", "marketplace/canary/.claude-plugin/marketplace.json", "main"},
	}
	for _, tt := range tests {
		t.Run("channel "+tt.channel, func(t *testing.T) {
			in := committedInput()
			in.Channel = tt.channel
			in.Pin = pinFor(claudeIndex)
			in.Pin.Ref = tt.ref

			d, err := Build(in)

			require.NoError(t, err)
			require.Contains(t, d.Files, tt.path)
			entries := pinnedEntries(t, d.Files[tt.path])
			assert.Equal(t, tt.ref, entries[0]["source"].(map[string]any)["ref"])
			assert.Contains(t, string(d.Files[SumsFile]), tt.path)
			for _, a := range d.Plan.Artifacts {
				if a.Path == tt.path {
					assert.Equal(t, "marketplace", a.Role)
				}
			}
			schemaValid(t, "publish-plan.schema.json", d.Files[PlanFile])
		})
	}
}

func TestBuild_PinnedIndexNeedsACleanCommit(t *testing.T) {
	in := committedInput()
	in.Pin = pinFor(claudeIndex)
	in.Source.Dirty = true

	_, err := Build(in)

	var pe *Error
	require.ErrorAs(t, err, &pe)
	assert.Equal(t, CodeSource, pe.Code)
}

func TestBuild_RejectsABadChannel(t *testing.T) {
	in := committedInput()
	in.Channel = "Bad Channel"

	_, err := Build(in)

	var pe *Error
	require.ErrorAs(t, err, &pe)
	assert.Equal(t, CodeConfig, pe.Code)
}

func emitInput(names ...string) *EmitRequest {
	return &EmitRequest{Names: names, Base: emit.Input{Market: emit.Market{Name: "acme-skills", OwnerName: "Acme"}}}
}

func TestBuild_RunsEmittersUnderEmitAndRefusesExperimentalOnes(t *testing.T) {
	cursor := committedInput()
	cursor.Files = append(cursor.Files, File{Path: ".cursor-plugin/plugin.json", Data: []byte("{}")})
	cursor.Emit = emitInput("cursor-team-marketplace")

	d, err := Build(cursor)

	require.NoError(t, err)
	assert.Contains(t, d.Files, "emit/cursor-team-marketplace/.cursor-plugin/marketplace.json")
	assert.Contains(t, d.Files, "emit/cursor-team-marketplace/plugins/acme/.cursor-plugin/plugin.json")
	assert.Empty(t, d.Warnings)

	in := committedInput()
	in.Emit = emitInput("port")
	_, err = Build(in)
	var pe *Error
	require.ErrorAs(t, err, &pe)
	assert.Equal(t, CodeConfig, pe.Code)
	assert.Contains(t, pe.Hint, "--experimental")

	in.Emit.Experimental = true
	d, err = Build(in)
	require.NoError(t, err)
	assert.Contains(t, d.Files, "emit/port/index.json")
	require.NotEmpty(t, d.Warnings)
	assert.Contains(t, d.Warnings[0], CodeExperimental)
}

func TestBuild_UnknownEmitter(t *testing.T) {
	in := committedInput()
	in.Emit = emitInput("jira")

	_, err := Build(in)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown emitter")
}

func TestBuild_EmitterOutputIsDeterministic(t *testing.T) {
	build := func() *Dist {
		in := committedInput()
		in.Emit = emitInput("kiro-steering", "aws-agent-registry", "port")
		in.Emit.Experimental = true
		d, err := Build(in)
		require.NoError(t, err)
		return d
	}

	a, b := build(), build()

	assert.Equal(t, a.Paths(), b.Paths())
	for _, p := range a.Paths() {
		assert.Equal(t, a.Files[p], b.Files[p], p)
	}
}

func TestDiffLocks(t *testing.T) {
	prev := &lockfile.File{
		Item: []lockfile.Item{
			{Kind: "skill", ID: "deploy", Digest: "sha256:aaaaaaaaaaaaaaaaaaaa"},
			{Kind: "rule", ID: "style", Domain: "backend", Digest: "sha256:bbbbbbbbbbbbbbbbbbbb"},
			{Kind: "agent", ID: "gone", Digest: "sha256:cccccccccccccccccccc"},
		},
		Include: []lockfile.Entry{{Name: "shared", Digest: "sha256:1", Commit: "c1"}},
	}
	cur := &lockfile.File{
		Item: []lockfile.Item{
			{Kind: "skill", ID: "deploy", Digest: "sha256:dddddddddddddddddddd"},
			{Kind: "rule", ID: "style", Domain: "backend", Digest: "sha256:bbbbbbbbbbbbbbbbbbbb"},
			{Kind: "skill", ID: "new", Digest: "sha256:eeeeeeeeeeeeeeeeeeee"},
		},
		Include: []lockfile.Entry{{Name: "shared", Digest: "sha256:1", Commit: "c2"}},
	}

	got := DiffLocks(prev, cur)

	assert.Equal(t, []NoteChange{
		{ChangeAdded, "skill", "", "new", "sha256:eeeeeeeeeeeeeeeeeeee"},
		{ChangeChanged, "include", "", "shared", "sha256:1"},
		{ChangeChanged, "skill", "", "deploy", "sha256:dddddddddddddddddddd"},
		{ChangeRemoved, "agent", "", "gone", "sha256:cccccccccccccccccccc"},
	}, got)
}

func lockWith(items ...lockfile.Item) []byte {
	return []byte("version = 2\ntree = \"sha256:x\"\n" + func() string {
		var sb strings.Builder
		for _, it := range items {
			sb.WriteString("\n[[item]]\nkind = \"" + it.Kind + "\"\nid = \"" + it.ID + "\"\ndigest = \"" + it.Digest + "\"\n")
		}
		return sb.String()
	}())
}

func TestBuild_ReleaseNotesListTheLockDiff(t *testing.T) {
	in := committedInput()
	in.Lock = lockWith(
		lockfile.Item{Kind: "skill", ID: "deploy", Digest: "sha256:" + strings.Repeat("d", 64)},
		lockfile.Item{Kind: "skill", ID: "added", Digest: "sha256:" + strings.Repeat("a", 64)},
	)
	in.PreviousLock = lockWith(
		lockfile.Item{Kind: "skill", ID: "deploy", Digest: "sha256:" + strings.Repeat("1", 64)},
		lockfile.Item{Kind: "rule", ID: "removed", Digest: "sha256:" + strings.Repeat("2", 64)},
	)
	in.PreviousLabel = "v1.3.0"

	d, err := Build(in)

	require.NoError(t, err)
	notes := string(d.Files[NotesFile])
	assert.Contains(t, notes, "## Changes since v1.3.0")
	assert.Contains(t, notes, "### Added\n\n- skill `added` aaaaaaaaaaaa")
	assert.Contains(t, notes, "### Changed\n\n- skill `deploy` dddddddddddd")
	assert.Contains(t, notes, "### Removed\n\n- rule `removed`")
	assert.NotContains(t, notes, strings.Repeat("d", 64), "digests are shortened")
}

func TestBuild_ReleaseNotesWithoutChanges(t *testing.T) {
	in := committedInput()
	in.PreviousLock, in.PreviousLabel = in.Lock, "v1.3.0"

	d, err := Build(in)

	require.NoError(t, err)
	assert.Contains(t, string(d.Files[NotesFile]), "No authored content or remote pin changed.")
}

func TestBuild_ReleaseNotesRejectAnUnreadablePreviousLock(t *testing.T) {
	in := committedInput()
	in.PreviousLock = []byte("not = [toml")

	_, err := Build(in)

	require.Error(t, err)
}

func TestRedact(t *testing.T) {
	tests := []struct{ in, notWant string }{
		{"auth failed with ghp_abcdefghijklmnopqrstuvwxyz0123456789", "ghp_abcdefghijklmnopqrstuvwxyz0123456789"},
		{"npm ERR! //registry.npmjs.org/:_authToken=npm_abcdefghijklmnopqrstuvwxyz123456", "npm_abcdefghijklmnopqrstuvwxyz123456"},
		{"Authorization: Bearer abcdef1234567890", "abcdef1234567890"},
		{"password=hunter2hunter2", "hunter2hunter2"},
	}
	for _, tt := range tests {
		got := Redact(tt.in)
		assert.NotContains(t, got, tt.notWant)
		assert.Contains(t, got, "[redacted]")
	}
	assert.Equal(t, "release v1.4.0 not found", Redact("release v1.4.0 not found"))
}

// ---- npm ----

func npmInput() Input {
	in := committedInput()
	in.Description = "Acme conventions"
	in.Repo = "acme/skills"
	in.Target = TargetNPM
	in.NPM = NPMOptions{Scope: "@acme"}
	return in
}

func TestValidateNPM(t *testing.T) {
	tests := []struct {
		name    string
		opts    NPMOptions
		plugin  string
		version string
		want    string
	}{
		{"ok", NPMOptions{Scope: "@acme"}, "conv", "1.4.0", ""},
		{"public with a registry", NPMOptions{Scope: "@acme", Access: "public", Registry: "https://npm.example.com"}, "conv", "1.4.0-rc.1", ""},
		{"no scope", NPMOptions{}, "conv", "1.4.0", "needs a scope"},
		{"scope without at", NPMOptions{Scope: "acme"}, "conv", "1.4.0", "needs a scope"},
		{"upper-case name", NPMOptions{Scope: "@acme"}, "Conv", "1.4.0", "not a valid npm package name"},
		{"not semver", NPMOptions{Scope: "@acme"}, "conv", "1.4", "semantic version"},
		{"bad access", NPMOptions{Scope: "@acme", Access: "all"}, "conv", "1.4.0", "invalid npm access"},
		{"http registry", NPMOptions{Scope: "@acme", Registry: "http://npm.example.com"}, "conv", "1.4.0", "https"},
		{"option-like scope", NPMOptions{Scope: "--registry=x"}, "conv", "1.4.0", "needs a scope"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := ValidateNPM(tt.opts, tt.plugin, tt.version)

			if tt.want != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.want)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "@acme/"+tt.plugin, plan.Package)
		})
	}
}

func TestBuild_NPMPlanAndPackage(t *testing.T) {
	in := npmInput()
	in.Channel = "canary"
	in.NPM.Access = "public"

	d, err := Build(in)

	require.NoError(t, err)
	require.NotNil(t, d.Plan.NPM)
	assert.Equal(t, "@acme/acme", d.Plan.NPM.Package)
	assert.Equal(t, "npm/acme-acme-1.4.0.tgz", d.Plan.NPM.Tarball)
	assert.Equal(t, []Command{
		{Argv: []string{"npm", "pack", "--ignore-scripts", "--pack-destination", "npm", "npm/package"}, Cwd: "."},
		{Argv: []string{"npm", "publish", "npm/acme-acme-1.4.0.tgz", "--access", "public", "--ignore-scripts", "--tag", "canary"}, Cwd: "."},
	}, d.Plan.Commands)
	var pkg map[string]any
	require.NoError(t, json.Unmarshal(d.Files["npm/package/package.json"], &pkg))
	assert.Equal(t, "@acme/acme", pkg["name"])
	assert.Equal(t, "1.4.0", pkg["version"])
	assert.Equal(t, []any{".claude-plugin", "hooks", "skills"}, pkg["files"], "the allow-list is the bundle's top-level entries")
	aiRulez := pkg["ai-rulez"].(map[string]any)
	assert.Equal(t, d.Manifest.Bundle.Digest, aiRulez["bundle_digest"])
	assert.Equal(t, map[string]any{"type": "git", "url": "git+https://github.com/acme/skills.git"}, pkg["repository"])
	assert.Equal(t, map[string]any{"access": "public"}, pkg["publishConfig"])
	assert.Contains(t, d.Files, "npm/package/skills/b/SKILL.md")
	schemaValid(t, "publish-plan.schema.json", d.Files[PlanFile])
	schemaValid(t, "publish-manifest.schema.json", d.Files["acme-1.4.0.manifest.json"])
}

func TestBuild_NPMRefusesABundleWithItsOwnPackageJSON(t *testing.T) {
	in := npmInput()
	in.Files = append(in.Files, File{Path: "package.json", Data: []byte("{}")})

	_, err := Build(in)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "already has a package.json")
}

func TestBuild_NPMNeedsAScope(t *testing.T) {
	in := npmInput()
	in.NPM = NPMOptions{}

	_, err := Build(in)

	var pe *Error
	require.ErrorAs(t, err, &pe)
	assert.Equal(t, CodeConfig, pe.Code)
}

func npmPlan(t *testing.T) Plan {
	t.Helper()
	d, err := Build(npmInput())
	require.NoError(t, err)
	return d.Plan
}

func TestExecuteNPM(t *testing.T) {
	notFound := runner.Result{Status: runner.StatusExit, ExitCode: 1, Stderr: []byte("npm ERR! code E404")}
	ok := runner.Result{Status: runner.StatusOK}
	tests := []struct {
		name      string
		view      runner.Result
		pack      runner.Result
		publish   runner.Result
		wantCalls []string
		wantErr   string
	}{
		{"packs then publishes", notFound, ok, ok, []string{"view", "pack", "publish"}, ""},
		{"refuses an existing version", runner.Result{Status: runner.StatusOK, Stdout: []byte("1.4.0\n")}, ok, ok, []string{"view"}, "already exists"},
		{"npm missing", runner.Result{Status: runner.StatusUnavailable, Err: os.ErrNotExist}, ok, ok, []string{"view"}, "npm was not found"},
		{"view fails for another reason", runner.Result{Status: runner.StatusExit, ExitCode: 1, Stderr: []byte("network down")}, ok, ok, []string{"view"}, "network down"},
		{"pack fails", notFound, runner.Result{Status: runner.StatusExit, ExitCode: 1, Stderr: []byte("bad package")}, ok, []string{"view", "pack"}, "bad package"},
		{"publish fails", notFound, ok, runner.Result{Status: runner.StatusExit, ExitCode: 1, Stderr: []byte("403 forbidden")}, []string{"view", "pack", "publish"}, "403 forbidden"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &runner.Fake{Handle: func(spec runner.Spec) runner.Result {
				switch spec.Argv[1] {
				case "view":
					return tt.view
				case "pack":
					return tt.pack
				}
				return tt.publish
			}}

			out, err := ExecuteNPM(context.Background(), fake, npmPlan(t), NPMExecuteOptions{Dir: "/d", Env: []string{"NODE_AUTH_TOKEN=placeholder"}})

			var verbs []string
			for _, c := range fake.Calls() {
				assert.Equal(t, "npm", c.Argv[0])
				assert.Equal(t, "/d", c.Dir)
				assert.Equal(t, []string{"NODE_AUTH_TOKEN=placeholder"}, c.Env)
				assert.False(t, c.InheritEnv)
				verbs = append(verbs, c.Argv[1])
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
			assert.Equal(t, "@acme/acme@1.4.0", out)
		})
	}
}

func TestExecuteNPM_RunsExactlyThePlannedArgvAndNeverEchoesATokenFromNPM(t *testing.T) {
	plan := npmPlan(t)
	fake := &runner.Fake{Handle: func(spec runner.Spec) runner.Result {
		switch spec.Argv[1] {
		case "view":
			return runner.Result{Status: runner.StatusExit, ExitCode: 1, Stderr: []byte("E404")}
		case "pack":
			return runner.Result{Status: runner.StatusOK}
		}
		return runner.Result{Status: runner.StatusExit, ExitCode: 1, Stderr: []byte("auth failed //registry.npmjs.org/:_authToken=npm_abcdefghijklmnopqrstuvwxyz123456")}
	}}

	_, err := ExecuteNPM(context.Background(), fake, plan, NPMExecuteOptions{})

	require.Error(t, err)
	assert.NotContains(t, err.Error(), "npm_abcdefghijklmnopqrstuvwxyz123456")
	calls := fake.Calls()
	require.Len(t, calls, 3)
	assert.Equal(t, plan.Commands[0].Argv, calls[1].Argv)
	assert.Equal(t, plan.Commands[1].Argv, calls[2].Argv)
}

func TestExecuteNPM_NeedsAnNPMPlan(t *testing.T) {
	_, err := ExecuteNPM(context.Background(), &runner.Fake{}, Plan{}, NPMExecuteOptions{})
	assert.Error(t, err)
}

func writeBuilt(t *testing.T, in Input) (string, *Dist) {
	t.Helper()
	d, err := Build(in)
	require.NoError(t, err)
	dir := filepath.Join(t.TempDir(), "dist")
	require.NoError(t, d.Write(dir))
	return dir, d
}

func TestVerify_ChecksTheNPMPlan(t *testing.T) {
	dir, _ := writeBuilt(t, npmInput())

	res, err := Verify(dir)

	require.NoError(t, err)
	assert.True(t, res.OK(), "%v", res.Problems)
}

func TestVerify_RejectsAnEditedNPMPlan(t *testing.T) {
	tests := []struct {
		name, old, repl string
	}{
		{"another registry", `"--ignore-scripts"`, `"--ignore-scripts", "--registry", "https://evil.example.com"`},
		{"another package", `"@acme/acme"`, `"@evil/acme"`},
		{"another tarball", `npm/acme-acme-1.4.0.tgz`, `npm/other.tgz`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, _ := writeBuilt(t, npmInput())
			replaceIn(t, filepath.Join(dir, PlanFile), tt.old, tt.repl)

			res, err := Verify(dir)

			require.NoError(t, err)
			assert.False(t, res.OK())
		})
	}
}

// ---- oci ----

func ociInput(repository string) Input {
	in := committedInput()
	in.Target = TargetOCI
	in.OCIRepository = repository
	in.Source.Repo = "https://github.com/acme/skills"
	return in
}

func newRegistry(t *testing.T) string {
	t.Helper()
	t.Setenv("DOCKER_CONFIG", t.TempDir()) // never read the developer's registry credentials
	srv := httptest.NewServer(registry.New(registry.Logger(log.New(io.Discard, "", 0))))
	t.Cleanup(srv.Close)
	return strings.TrimPrefix(srv.URL, "http://")
}

func TestBuild_OCIPlanRecordsTheManifestDigest(t *testing.T) {
	d, err := Build(ociInput("ghcr.io/acme/skills/acme"))

	require.NoError(t, err)
	assert.Equal(t, "ghcr.io/acme/skills/acme:1.4.0", d.Plan.Ref)
	assert.Equal(t, Digest(d.Files[OCIManifestFile]), d.Plan.OCIDigest)
	assert.Equal(t, []string{"acme-1.4.0.tar.gz", "ai-rulez.lock"}, d.Plan.Upload)
	assert.Empty(t, d.Plan.Commands, "the push is in process: no argv")
	assert.Contains(t, string(d.Files[OCIManifestFile]), oci.ArtifactType)
	schemaValid(t, "publish-plan.schema.json", d.Files[PlanFile])
}

func TestBuild_OCIManifestIsReproducible(t *testing.T) {
	a, err := Build(ociInput("ghcr.io/acme/skills/acme"))
	require.NoError(t, err)
	b, err := Build(ociInput("ghcr.io/acme/skills/acme"))
	require.NoError(t, err)

	assert.Equal(t, a.Plan.OCIDigest, b.Plan.OCIDigest)
}

func TestBuild_OCIRejectsBadRepositories(t *testing.T) {
	for _, repo := range []string{"", "acme", "ghcr.io/Acme/Skills", "ghcr.io/acme/skills:1.0", "ghcr.io/acme/skills@sha256:" + strings.Repeat("a", 64)} {
		t.Run(repo, func(t *testing.T) {
			_, err := Build(ociInput(repo))

			var pe *Error
			require.ErrorAs(t, err, &pe)
			assert.Equal(t, CodeConfig, pe.Code)
		})
	}
}

func TestOCITag(t *testing.T) {
	assert.Equal(t, "1.4.0_build.5", OCITag("1.4.0+build.5"))
	ref, err := ociReference("ghcr.io/acme/x", "1.4.0+build.5")
	require.NoError(t, err)
	assert.Equal(t, "ghcr.io/acme/x:1.4.0_build.5", ref)
}

func TestExecuteOCI_PushesTheReviewedArtifactToALocalRegistry(t *testing.T) {
	host := newRegistry(t)
	dir, d := writeBuilt(t, ociInput(host+"/acme/skills/acme"))

	pushed, err := ExecuteOCI(context.Background(), d.Plan, OCIExecuteOptions{Dir: dir})

	require.NoError(t, err)
	assert.Equal(t, host+"/acme/skills/acme@"+d.Plan.OCIDigest, pushed)
	got, err := oci.Pull(context.Background(), oci.Target{Ref: host + "/acme/skills/acme:1.4.0"})
	require.NoError(t, err)
	assert.Equal(t, d.Plan.OCIDigest, got.Digest)
	assert.Equal(t, d.Files["acme-1.4.0.manifest.json"], got.Config)
	assert.Equal(t, d.Files["acme-1.4.0.tar.gz"], got.Layers[0].Data)
}

func TestExecuteOCI_RefusesFilesThatNoLongerMatchThePlan(t *testing.T) {
	host := newRegistry(t)
	dir, d := writeBuilt(t, ociInput(host+"/acme/skills/acme"))
	appendTo(t, filepath.Join(dir, LockFile), "# edited\n")

	_, err := ExecuteOCI(context.Background(), d.Plan, OCIExecuteOptions{Dir: dir})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no longer match the plan")
	_, perr := oci.Pull(context.Background(), oci.Target{Ref: host + "/acme/skills/acme:1.4.0"})
	assert.Error(t, perr, "nothing was pushed")
}

func TestExecuteOCI_NeedsAnOCIPlan(t *testing.T) {
	_, err := ExecuteOCI(context.Background(), Plan{}, OCIExecuteOptions{})
	assert.Error(t, err)
}

func TestExecuteOCI_ReportsAnUnreachableRegistryWithoutSecrets(t *testing.T) {
	t.Setenv("DOCKER_CONFIG", t.TempDir())
	dir, d := writeBuilt(t, ociInput("127.0.0.1:1/acme/skills/acme"))

	_, err := ExecuteOCI(context.Background(), d.Plan, OCIExecuteOptions{Dir: dir})

	var pe *Error
	require.ErrorAs(t, err, &pe)
	assert.Equal(t, CodeTarget, pe.Code)
}

func TestVerify_ChecksTheOCIPlanAndPulledArtifact(t *testing.T) {
	host := newRegistry(t)
	dir, d := writeBuilt(t, ociInput(host+"/acme/skills/acme"))
	res, err := Verify(dir)
	require.NoError(t, err)
	require.True(t, res.OK(), "%v", res.Problems)
	_, err = ExecuteOCI(context.Background(), d.Plan, OCIExecuteOptions{Dir: dir})
	require.NoError(t, err)

	pulled := t.TempDir()
	require.NoError(t, PullOCI(context.Background(), oci.Target{Ref: host + "/acme/skills/acme:1.4.0"}, pulled))
	got, err := Verify(pulled)

	require.NoError(t, err)
	assert.True(t, got.OK(), "%v", got.Problems)
	assert.Equal(t, "acme", got.Name)
}

func TestVerify_RejectsATamperedOCIManifest(t *testing.T) {
	dir, _ := writeBuilt(t, ociInput("ghcr.io/acme/skills/acme"))
	replaceIn(t, filepath.Join(dir, OCIManifestFile), "application/vnd.ai-rulez.bundle.v1.tar+gzip", "application/octet-stream")

	res, err := Verify(dir)

	require.NoError(t, err)
	assert.False(t, res.OK())
}

// ---- signing, SBOM, approval ----

func keyPair(t *testing.T) (signing.Signer, VerifyOptions) {
	t.Helper()
	priv, pub, err := signing.GenerateKeyPair([]byte("pw"))
	require.NoError(t, err)
	signer, err := signing.LoadKeySigner(priv, []byte("pw"))
	require.NoError(t, err)
	key, err := signing.ParsePublicKey(pub)
	require.NoError(t, err)
	return signer, VerifyOptions{Keys: []crypto.PublicKey{key}, Now: time.Now()}
}

func signedInput(t *testing.T, signer signing.Signer) Input {
	t.Helper()
	in := committedInput()
	in.Sign = func(archive []byte) (*SignResult, error) { return SignArchive(context.Background(), signer, archive) }
	return in
}

func TestBuild_SignsTheArchiveAndRecordsTheSigner(t *testing.T) {
	signer, trust := keyPair(t)

	d, err := Build(signedInput(t, signer))

	require.NoError(t, err)
	require.NotNil(t, d.Manifest.Signature)
	assert.Equal(t, "acme-1.4.0.tar.gz.sigstore.json", d.Manifest.Signature.File)
	assert.Equal(t, "key", d.Manifest.Signature.Signer.Kind)
	assert.Regexp(t, `^sha256:[0-9a-f]{64}$`, d.Manifest.Signature.Signer.KeyID)
	assert.Contains(t, d.Files, "acme-1.4.0.tar.gz.sigstore.json")
	assert.Contains(t, string(d.Files[SumsFile]), "acme-1.4.0.tar.gz.sigstore.json")
	_, err = VerifyArchiveSignature(d.Files["acme-1.4.0.tar.gz.sigstore.json"], d.Files["acme-1.4.0.tar.gz"], trust)
	require.NoError(t, err, "the bundle verifies as a message signature over the archive bytes")
	schemaValid(t, "publish-manifest.schema.json", d.Files["acme-1.4.0.manifest.json"])
	schemaValid(t, "publish-plan.schema.json", d.Files[PlanFile])
	assert.Contains(t, string(d.Files[NotesFile]), "Signature:")
}

func TestBuild_SignedReleaseIsUploadedWithItsSignature(t *testing.T) {
	signer, _ := keyPair(t)
	in := signedInput(t, signer)
	in.Target, in.Tag, in.Repo = TargetGitHubRelease, "v1.4.0", "acme/skills"

	d, err := Build(in)

	require.NoError(t, err)
	assert.Equal(t, []string{"acme-1.4.0.tar.gz", "acme-1.4.0.manifest.json", "ai-rulez.lock", "SHA256SUMS", "acme-1.4.0.tar.gz.sigstore.json"}, d.Plan.Upload)
}

func TestBuild_RequireSignatureNeedsASigner(t *testing.T) {
	in := committedInput()
	in.RequireSignature = true

	_, err := Build(in)

	var pe *Error
	require.ErrorAs(t, err, &pe)
	assert.Equal(t, CodeUnsigned, pe.Code)
	assert.Equal(t, ExitGate, pe.Exit)
}

func TestBuild_ASigningFailureStopsTheBuild(t *testing.T) {
	in := committedInput()
	in.Sign = func([]byte) (*SignResult, error) { return nil, os.ErrPermission }

	_, err := Build(in)

	require.Error(t, err)
}

func TestVerifyWith_Signature(t *testing.T) {
	signer, trust := keyPair(t)
	_, otherTrust := keyPair(t)
	dir, _ := writeBuilt(t, signedInput(t, signer))
	unsignedDir, _ := writeBuilt(t, committedInput())

	tests := []struct {
		name      string
		dir       string
		checks    VerifyChecks
		wantState string
		wantProb  string
	}{
		{"trusted signer", dir, VerifyChecks{Signature: trust, RequireSignature: true}, "verified", ""},
		{"signed, no signer named", dir, VerifyChecks{}, "unverified", ""},
		{"signed, none named, required", dir, VerifyChecks{RequireSignature: true}, "unverified", "no trusted key or identity"},
		{"another key", dir, VerifyChecks{Signature: otherTrust}, "unverified", "does not verify"},
		{"unsigned", unsignedDir, VerifyChecks{}, "none", ""},
		{"unsigned, required", unsignedDir, VerifyChecks{RequireSignature: true}, "none", "not signed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := VerifyWith(tt.dir, tt.checks)

			require.NoError(t, err)
			assert.Equal(t, tt.wantState, res.Signature)
			if tt.wantProb == "" {
				assert.True(t, res.OK(), "%v", res.Problems)
				return
			}
			require.False(t, res.OK())
			assert.Contains(t, res.Problems[0].Message, tt.wantProb)
			assert.Contains(t, res.Problems[0].Message, CodeUnsigned)
		})
	}
}

func TestVerifyWith_ATamperedArchiveFailsTheSignatureToo(t *testing.T) {
	signer, trust := keyPair(t)
	dir, d := writeBuilt(t, signedInput(t, signer))
	tampered := append([]byte(nil), d.Files["acme-1.4.0.tar.gz"]...)
	tampered[len(tampered)-1] ^= 0xff
	require.NoError(t, os.WriteFile(filepath.Join(dir, "acme-1.4.0.tar.gz"), tampered, 0o600))

	res, err := VerifyWith(dir, VerifyChecks{Signature: trust})

	require.NoError(t, err)
	assert.False(t, res.OK())
}

func TestVerify_ASignatureTheManifestNamesMustBeListed(t *testing.T) {
	signer, _ := keyPair(t)
	dir, _ := writeBuilt(t, signedInput(t, signer))
	require.NoError(t, os.Remove(filepath.Join(dir, "acme-1.4.0.tar.gz.sigstore.json")))

	res, err := Verify(dir)

	require.NoError(t, err)
	assert.False(t, res.OK())
}

func TestBuild_SBOMSlotIsFilledAndVerified(t *testing.T) {
	in := committedInput()
	in.SBOM = []byte(`{"bomFormat":"CycloneDX","specVersion":"1.6"}` + "\n")
	dir, d := writeBuilt(t, in)

	require.NotNil(t, d.Manifest.SBOM)
	assert.Equal(t, "cyclonedx", d.Manifest.SBOM.Format)
	assert.Equal(t, "acme-1.4.0.sbom.cdx.json", d.Manifest.SBOM.File)
	assert.Equal(t, Digest(in.SBOM), d.Manifest.SBOM.Digest)
	schemaValid(t, "publish-manifest.schema.json", d.Files["acme-1.4.0.manifest.json"])
	res, err := Verify(dir)
	require.NoError(t, err)
	assert.True(t, res.OK(), "%v", res.Problems)

	appendTo(t, filepath.Join(dir, "acme-1.4.0.sbom.cdx.json"), " ")
	res, err = Verify(dir)
	require.NoError(t, err)
	assert.False(t, res.OK())
}

func TestBuild_ApprovalSummaryIsRecorded(t *testing.T) {
	in := committedInput()
	in.Approval = &ApprovalInfo{Required: 3, Approved: 3}

	d, err := Build(in)

	require.NoError(t, err)
	assert.Equal(t, &ApprovalInfo{Required: 3, Approved: 3}, d.Manifest.Approval)
	schemaValid(t, "publish-manifest.schema.json", d.Files["acme-1.4.0.manifest.json"])
}

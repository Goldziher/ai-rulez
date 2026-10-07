package generator

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
)

const driftIgnoring = `version = "5.0"
name = "shared-project"
presets = ["claude"]
`

// subProject creates a drift project rooted at base/sub (sharing base with other projects).
func subProject(t *testing.T, base, sub, shared string) *driftProject {
	t.Helper()
	root := filepath.Join(base, filepath.FromSlash(sub))
	dir := filepath.Join(root, ".ai-rulez")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "rules"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.toml"), []byte(shared), 0o600))
	return &driftProject{base: root, dir: dir}
}

func readExclude(t *testing.T, repo *driftProject) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repo.base, ".git", "info", "exclude"))
	if err != nil {
		return ""
	}
	return string(data)
}

func TestLocalRuleFilesClassifyAsLocalOnlyNotDrift(t *testing.T) {
	// Arrange
	p := newDriftProject(t, driftShared)
	require.NoError(t, os.MkdirAll(filepath.Join(p.dir, "local", "rules"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(p.dir, "local", "rules", "mine.md"),
		[]byte("---\npriority: high\n---\n\nMy private rule.\n"), 0o600))
	gen := NewGenerator(p.load(t))

	// Act
	plan, err := gen.DryRun("")

	// Assert
	require.NoError(t, err)
	var localOnly, drift []string
	for _, line := range plan {
		switch {
		case strings.HasPrefix(line, "local-only: "):
			localOnly = append(localOnly, strings.TrimPrefix(line, "local-only: "))
		case strings.HasPrefix(line, "drift: "):
			drift = append(drift, strings.TrimPrefix(line, "drift: "))
		}
	}
	require.NotEmpty(t, localOnly, "local content produces local-only outputs: %v", plan)
	assert.True(t, containsSub(localOnly, ".local"), "a local rule file is among them: %v", localOnly)
	for _, rel := range drift {
		assert.NotContains(t, rel, "mine", "a local rule is never reported as drift")
	}
	assert.False(t, containsPrefix(plan, "blocked:"), "local-only outputs do not block: %v", plan)
}

func containsSub(list []string, sub string) bool {
	for _, s := range list {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func TestExclude_TwoWorktreesWithDifferentOverlaysKeepTheirOwnBlocks(t *testing.T) {
	// Arrange: a committed project checked out twice.
	main := newDriftProject(t, driftIgnoring)
	main.git(t, "init", "-q")
	main.git(t, "add", ".ai-rulez")
	main.git(t, "commit", "-q", "-m", "init")
	linked := filepath.Join(t.TempDir(), "wt")
	main.git(t, "worktree", "add", "-q", linked, "-b", "feature")
	wt := &driftProject{base: linked, dir: filepath.Join(linked, ".ai-rulez")}
	main.overlay(t, "presets = [\"codex\"]\n")
	wt.overlay(t, "presets = [\"gemini\"]\n")

	// Act
	require.NoError(t, NewGenerator(main.load(t)).Generate(""))
	require.NoError(t, NewGenerator(wt.load(t)).Generate(""))
	require.NoError(t, NewGenerator(main.load(t)).Generate(""))

	// Assert: each worktree's block survives the other's runs.
	exclude := readExclude(t, main)
	assert.Contains(t, exclude, "/AGENTS.md")
	assert.Contains(t, exclude, "/GEMINI.md")
	assert.Equal(t, 2, strings.Count(exclude, "# BEGIN ai-rulez local: "), exclude)
}

func TestExclude_SubProjectsInOneRepositoryDoNotClobberEachOther(t *testing.T) {
	// Arrange
	repo := t.TempDir()
	top := &driftProject{base: repo}
	top.git(t, "init", "-q")
	a := subProject(t, repo, "pkg/a", driftIgnoring)
	b := subProject(t, repo, "pkg/b", driftIgnoring)
	a.overlay(t, "presets = [\"codex\"]\n")
	b.overlay(t, "presets = [\"gemini\"]\n")

	// Act
	require.NoError(t, NewGenerator(a.load(t)).Generate(""))
	require.NoError(t, NewGenerator(b.load(t)).Generate(""))
	require.NoError(t, NewGenerator(a.load(t)).Generate(""))

	// Assert: patterns are anchored at the repository root, and both blocks remain.
	exclude := readExclude(t, top)
	assert.Contains(t, exclude, "/pkg/a/AGENTS.md")
	assert.Contains(t, exclude, "/pkg/b/GEMINI.md")
	assert.NotContains(t, exclude, "\n/AGENTS.md", "a sub-project's pattern is not anchored at the root")
}

func TestExclude_BlockIsRemovedWhenTheOverlayGoesAway(t *testing.T) {
	// Arrange
	p := newDriftProject(t, driftIgnoring)
	p.git(t, "init", "-q")
	other := "# my own exclude\n/scratch/\n"
	require.NoError(t, os.MkdirAll(filepath.Join(p.base, ".git", "info"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(p.base, ".git", "info", "exclude"), []byte(other), 0o600))
	p.overlay(t, "presets = [\"codex\"]\n")
	require.NoError(t, NewGenerator(p.load(t)).Generate(""))
	require.Contains(t, readExclude(t, p), "/AGENTS.md")

	// Act: the overlay is deleted and a plain run follows.
	require.NoError(t, os.Remove(filepath.Join(p.dir, "config.local.toml")))
	require.NoError(t, NewGenerator(p.load(t)).Generate(""))

	// Assert
	exclude := readExclude(t, p)
	assert.NotContains(t, exclude, "AGENTS.md")
	assert.NotContains(t, exclude, "BEGIN ai-rulez local")
	assert.Contains(t, exclude, "/scratch/", "entries ai-rulez did not write are never dropped")
}

func TestExclude_SkippedLocalRunLeavesTheBlockAlone(t *testing.T) {
	// Arrange
	p := newDriftProject(t, driftIgnoring)
	p.git(t, "init", "-q")
	p.overlay(t, "presets = [\"codex\"]\n")
	require.NoError(t, NewGenerator(p.load(t)).Generate(""))
	before := readExclude(t, p)

	// Act
	require.NoError(t, NewGenerator(p.load(t, config.WithoutLocal())).Generate(""))

	// Assert
	assert.Equal(t, before, readExclude(t, p))
}

func TestEscapeGitPattern(t *testing.T) {
	tests := []struct{ in, want string }{
		{"plain/path.md", "plain/path.md"},
		{"dir/a[1]*.md", `dir/a\[1\]\*.md`},
		{"what?.md", `what\?.md`},
		{"#hash.md", `\#hash.md`},
		{"!bang.md", `\!bang.md`},
		{`back\slash.md`, `back\\slash.md`},
		{"trailing  ", `trailing\ \ `},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			assert.Equal(t, tt.want, escapeGitPattern(tt.in))
		})
	}
}

func TestEscapeGitPattern_GitAgrees(t *testing.T) {
	// Arrange: files whose names are full of gitignore syntax.
	p := newDriftProject(t, driftIgnoring)
	p.git(t, "init", "-q")
	names := []string{"dir/a[1]*.md", "#hash.md", "!bang.md", "what?.md", "sp ace.md", "other.md"}
	var patterns []string
	for _, n := range names[:5] {
		patterns = append(patterns, "/"+escapeGitPattern(n))
	}
	require.NoError(t, os.MkdirAll(filepath.Join(p.base, ".git", "info"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(p.base, ".git", "info", "exclude"), []byte(strings.Join(patterns, "\n")+"\n"), 0o600))

	// Act
	ignored, err := gitutil.IgnoredAmong(p.base, names)

	// Assert: exactly the five listed files are ignored, and nothing else is caught.
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{
		"dir/a[1]*.md": true, "#hash.md": true, "!bang.md": true, "what?.md": true, "sp ace.md": true,
	}, ignored)
}

func TestBaselineFailure_FailsClosedWithAndWithoutTheFlag(t *testing.T) {
	// Arrange: a profile that exists only in the overlay cannot be rendered as the
	// shared baseline.
	p := newDriftProject(t, driftIgnoring)
	require.NoError(t, os.MkdirAll(filepath.Join(p.dir, "domains", "d", "rules"), 0o755))
	p.overlay(t, "[profiles]\nonlylocal = [\"d\"]\n")
	require.NoError(t, os.MkdirAll(filepath.Join(p.dir, "local", "rules"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(p.dir, "local", "rules", "mine.md"), []byte("---\npriority: high\n---\n\nPrivate.\n"), 0o600))

	// Act
	withoutFlag := NewGenerator(p.load(t)).Generate("onlylocal")
	allowing := NewGenerator(p.load(t))
	allowing.SetAllowLocalDrift(true)
	withFlag := allowing.Generate("onlylocal")

	// Assert
	require.Error(t, withoutFlag)
	assert.Contains(t, withoutFlag.Error(), "shared baseline")
	// The flag does not bypass a failed baseline: overlay-derived outputs could
	// otherwise land in tracked files unmarked.
	require.Error(t, withFlag)
	assert.Contains(t, withFlag.Error(), "shared baseline")
	assert.Contains(t, withFlag.Error(), "--no-local")
}

func TestRedactCredentials(t *testing.T) {
	err := redactCredentials(assertError("fetch https://x-access-token:ghp_secret@github.com/o/r.git failed"))

	assert.NotContains(t, err.Error(), "ghp_secret")
	assert.Contains(t, err.Error(), "github.com/o/r.git")
	plain := assertError("no url here")
	assert.Equal(t, plain, redactCredentials(plain))
}

type assertError string

func (e assertError) Error() string { return string(e) }

func TestLocalSourceHash_UsesARedactedCanonicalOverlay(t *testing.T) {
	hash := func(doc map[string]any) string {
		cfg := &config.Config{LocalOverlay: &config.LocalOverlay{Doc: doc}}
		h, err := NewGenerator(cfg).localSourceHash("baseline")
		require.NoError(t, err)
		return h
	}
	server := func(token, header, arg, url string, command string) map[string]any {
		return map[string]any{"mcp_servers": []any{map[string]any{
			"name": "svc", "command": command, "args": []any{arg}, "url": url,
			"env":     map[string]any{"API_TOKEN": token},
			"headers": map[string]any{"Authorization": header},
		}}}
	}
	base := hash(server("t1", "Bearer a", "--key=1", "https://user:pw@example.com/mcp?token=q", "svc"))

	assert.Equal(t, base, hash(server("t2", "Bearer b", "--key=2", "https://other:pw2@example.com/mcp?token=r", "svc")),
		"secrets, args, credentials and queries do not enter the hash")
	assert.NotEqual(t, base, hash(server("t1", "Bearer a", "--key=1", "https://user:pw@example.com/mcp?token=q", "different")),
		"structure still does")
	assert.NotEqual(t, base, hash(server("t1", "Bearer a", "--key=1", "https://user:pw@elsewhere.com/mcp", "svc")),
		"the host is part of the structure")
}

func TestCanonicalOverlay_ContainsNoSecretText(t *testing.T) {
	doc := map[string]any{
		"includes": []any{map[string]any{"name": "priv", "source": "https://x-access-token:ghp_abc@github.com/o/r.git"}},
		"mcp_servers": []any{map[string]any{
			"name": "svc", "args": []any{"--token", "tok-123"}, "url": "https://u:p@example.com/x?key=k9",
			"env": map[string]any{"PLAIN": "env-secret"}, "headers": map[string]any{"X-Thing": "hdr-secret"},
		}},
	}

	rendered := canonicalString(nil, "")
	data := mustJSONString(t, canonicalOverlay(nil, doc))

	for _, secret := range []string{"ghp_abc", "tok-123", "u:p", "k9", "env-secret", "hdr-secret"} {
		assert.NotContains(t, data, secret)
	}
	assert.Empty(t, rendered)
	assert.Contains(t, data, "github.com/o/r.git")
}

func TestGenerate_GitFailureFailsClosedForTheGuard(t *testing.T) {
	// Arrange: a repository whose index is unreadable, so git cannot say what is tracked.
	p := newDriftProject(t, strings.Replace(driftShared, "gitignore = false", "gitignore = true", 1))
	p.git(t, "init", "-q")
	require.NoError(t, os.WriteFile(filepath.Join(p.base, ".git", "index"), []byte("garbage"), 0o600))
	p.overlay(t, "name = \"mine\"\npresets = [\"codex\"]\n")

	// Act
	err := NewGenerator(p.load(t)).Generate("")

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "local overrides would change")
}

func TestGenerate_TrackedLocalOnlyOutputIsAViolation(t *testing.T) {
	// Arrange: AGENTS.md is already committed, then a local preset starts producing it.
	p := newDriftProject(t, strings.Replace(driftShared, "gitignore = false", "gitignore = true", 1))
	p.git(t, "init", "-q")
	require.NoError(t, os.WriteFile(filepath.Join(p.base, "AGENTS.md"), []byte("hand written\n"), 0o600))
	p.git(t, "add", "-f", "AGENTS.md")
	p.overlay(t, "presets = [\"codex\"]\n")

	// Act
	err := NewGenerator(p.load(t)).Generate("")

	// Assert
	require.Error(t, err)
	assert.Equal(t, "hand written\n", p.read(t, "AGENTS.md"))
}

func TestGenerate_WithoutGitTheGuardTreatsNothingAsTracked(t *testing.T) {
	// Arrange: no git at all on PATH.
	t.Setenv("PATH", t.TempDir())
	p := newDriftProject(t, strings.Replace(driftShared, "gitignore = false", "gitignore = true", 1))
	p.overlay(t, "presets = [\"codex\"]\n")

	// Act
	err := NewGenerator(p.load(t)).Generate("")

	// Assert: local-only output is fine and lands in .gitignore as the fallback.
	require.NoError(t, err)
	assert.Contains(t, p.read(t, ".gitignore"), "AGENTS.md")
}

func TestGenerate_DriftFileThatWasPreviouslyLocalOnly(t *testing.T) {
	// Arrange: AGENTS.md first exists only because of the overlay (codex preset);
	// later the shared config adopts codex too, so the file is shared but differs.
	p := newDriftProject(t, strings.Replace(driftShared, "gitignore = false", "gitignore = true", 1))
	p.overlay(t, "presets = [\"codex\"]\nname = \"mine\"\n")
	gen := NewGenerator(p.load(t))
	gen.SetAllowLocalDrift(true)
	require.NoError(t, gen.Generate(""))

	// Act
	shared := strings.Replace(driftShared, `presets = ["claude"]`, `presets = ["claude", "codex"]`, 1)
	require.NoError(t, os.WriteFile(filepath.Join(p.dir, "config.toml"), []byte(shared), 0o600))
	plan, err := NewGenerator(p.load(t)).DryRun("")

	// Assert
	require.NoError(t, err)
	assert.Contains(t, plan, "drift: AGENTS.md")
	assert.NotContains(t, plan, "local-only: AGENTS.md")
}

func TestGenerate_PartiallyOwnedMCPDocumentDrift(t *testing.T) {
	// Arrange: a tracked .mcp.json holds hand-authored settings; a local server would be merged into it.
	shared := strings.Replace(driftShared, "gitignore = false", "gitignore = true", 1) + `
[[mcp_servers]]
name = "shared-svc"
command = "svc"
`
	p := newDriftProject(t, shared)
	p.git(t, "init", "-q")
	require.NoError(t, NewGenerator(p.load(t, config.WithoutLocal())).Generate(""))
	p.git(t, "add", "-f", ".mcp.json")
	p.overlay(t, "[[mcp_servers]]\nname = \"mine\"\ncommand = \"mine\"\n")

	// Act
	err := NewGenerator(p.load(t)).Generate("")

	// Assert
	require.Error(t, err)
	assert.NotContains(t, p.read(t, ".mcp.json"), "\"mine\"")
}

func TestGenerate_SecretsNeverLeakIntoSharedArtifacts(t *testing.T) {
	// Arrange: an overlay with MCP env and header secrets and a credentialed include.
	shared := strings.Replace(driftShared, "gitignore = false", "gitignore = true", 1)
	p := newDriftProject(t, shared)
	p.git(t, "init", "-q")
	require.NoError(t, os.MkdirAll(filepath.Join(p.base, "inc", ".ai-rulez"), 0o755))
	p.overlay(t, `presets = ["codex"]

[[mcp_servers]]
name = "svc"
transport = "http"
url = "https://example.com/mcp"
[mcp_servers.headers]
Authorization = "Bearer HEADER-SECRET-1"

[[mcp_servers]]
name = "tool"
command = "tool"
args = ["--key", "ARG-SECRET-2"]
[mcp_servers.env]
API_TOKEN = "ENV-SECRET-3"
`)
	gen := NewGenerator(p.load(t))

	// Act
	dry, dryErr := gen.DryRun("")
	genErr := NewGenerator(p.load(t)).Generate("")

	// Assert
	require.NoError(t, dryErr)
	require.NoError(t, genErr)
	secrets := []string{"HEADER-SECRET-1", "ARG-SECRET-2", "ENV-SECRET-3"}
	artifacts := map[string]string{
		"dry-run":            strings.Join(dry, "\n"),
		"committed manifest": p.read(t, ".ai-rulez/.generated-manifest.json"),
		".gitignore":         p.read(t, ".gitignore"),
		"info/exclude":       readExclude(t, p),
		"CLAUDE.md":          p.read(t, "CLAUDE.md"),
		"AGENTS.md":          p.read(t, "AGENTS.md"),
	}
	for name, content := range artifacts {
		for _, secret := range secrets {
			assert.NotContains(t, content, secret, "%s must not carry %s", name, secret)
		}
	}
	// And an error about drift never prints content either.
	p.overlay(t, "name = \"DRIFT-NAME-4\"\n[[mcp_servers]]\nname = \"x\"\ncommand = \"c\"\n[mcp_servers.env]\nAPI_TOKEN = \"ENV-SECRET-5\"\n")
	p2 := newDriftProject(t, driftShared)
	p2.overlay(t, "name = \"DRIFT-NAME-4\"\n[[mcp_servers]]\nname = \"x\"\ncommand = \"c\"\n[mcp_servers.env]\nAPI_TOKEN = \"ENV-SECRET-5\"\n")
	err := NewGenerator(p2.load(t)).Generate("")
	require.Error(t, err)
	rendered := err.Error() + strings.Join(errorLines(err), "\n")
	assert.NotContains(t, rendered, "DRIFT-NAME-4")
	assert.NotContains(t, rendered, "ENV-SECRET-5")
}

func TestBaselineLoad_HonoursTheCallersContext(t *testing.T) {
	// Arrange: a canceled caller context must reach the baseline load.
	p := newDriftProject(t, driftShared)
	p.overlay(t, "presets = [\"codex\"]\n")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	gen := NewGenerator(p.load(t))
	gen.SetContext(ctx)

	// Act: the baseline render itself does not fail on a canceled context for
	// local content, so assert the context is the one the generator holds.
	assert.Equal(t, ctx, gen.context())
	assert.Equal(t, context.Background(), NewGenerator(p.load(t)).context())
}

func mustJSONString(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	return string(data)
}

func TestPlanLocal_PendingPatternsAreComputedFromThePlan(t *testing.T) {
	// Arrange: the overlay adds codex, whose AGENTS.md exists only on this machine
	// and is excluded through .git/info/exclude, not the shared .gitignore block.
	p := newDriftProject(t, strings.Replace(driftShared, "gitignore = false", "gitignore = true", 1))
	p.overlay(t, "presets = [\"codex\"]\n")
	g := NewGenerator(p.load(t))
	merged, _, err := g.collectOutputs("")
	require.NoError(t, err)

	// Act
	plan, err := g.planLocal("", merged)

	// Assert: the plan is the generator's own, so the patterns the violation scan
	// saw leave out the machine-local file.
	require.NoError(t, err)
	require.NotNil(t, plan)
	assert.Same(t, plan, g.plan)
	require.True(t, plan.machineLocal["AGENTS.md"])
	assert.NotContains(t, g.pendingIgnorePatterns(merged), "AGENTS.md")
}

package sessionstart

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
	itestutil "github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

const fakeExe = "/fake/ai-rulez"

type harness struct {
	t      *testing.T
	root   string
	cfg    *config.Config
	runner *runner.Fake
	env    map[string]string
	deps   Deps
	out    bytes.Buffer
	errw   bytes.Buffer
}

// newHarness builds a git work tree with an ai-rulez config and fake dependencies.
func newHarness(t *testing.T, session *config.SessionStartConfig) *harness {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".git"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".ai-rulez"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".ai-rulez", "config.toml"), []byte("version = \"5.0\"\n"), 0o644))
	h := &harness{t: t, root: root, runner: &runner.Fake{}, env: map[string]string{config.SessionStartRemoteEnv: "true"}}
	h.cfg = &config.Config{
		Version: "5.0", Name: "t", Presets: []config.Preset{{BuiltIn: "claude"}},
		ConfigDir: filepath.Join(root, ".ai-rulez"), ConfigDirName: ".ai-rulez", SessionStart: session,
	}
	h.deps = Deps{
		Getenv:  func(k string) string { return h.env[k] },
		Environ: func() []string { return h.environ() },
		Getwd:   func() (string, error) { return root, nil },
		Runner:  h.runner,
		Exe:     fakeExe,
		TempDir: t.TempDir(),
		UID:     os.Getuid(),
		Now:     time.Now,
		LoadConfig: func(context.Context, string) (*config.Config, error) {
			return h.cfg, nil
		},
	}
	return h
}

func (h *harness) environ() []string {
	var out []string
	for k, v := range h.env {
		out = append(out, k+"="+v)
	}
	return append(out, "PATH=/usr/bin", "HOME=/home/x")
}

func (h *harness) run(payload string, ov Overrides) int {
	h.t.Helper()
	return Run(context.Background(), strings.NewReader(payload), &h.out, &h.errw, h.deps, ov)
}

func (h *harness) writeLock() {
	h.t.Helper()
	require.NoError(h.t, os.WriteFile(filepath.Join(h.cfg.ConfigDir, lockfile.FileName), []byte("x"), 0o644))
}

func (h *harness) argvs() [][]string {
	var out [][]string
	for _, c := range h.runner.Calls() {
		out = append(out, c.Argv)
	}
	return out
}

func off() *bool { b := false; return &b }

func failing(stderr string) func(runner.Spec) runner.Result {
	return func(runner.Spec) runner.Result {
		return runner.Result{Status: runner.StatusExit, ExitCode: 2, Stderr: []byte(stderr)}
	}
}

func TestRun_ModesAndSources(t *testing.T) {
	base := []string{fakeExe, "generate", "--yes", "--quiet"}
	tests := []struct {
		name     string
		session  *config.SessionStartConfig
		lock     bool
		payload  string
		ov       Overrides
		remote   bool
		want     [][]string
		wantJSON bool
	}{
		{name: "not remote is a silent no-op", payload: `{"source":"startup"}`},
		{name: "remote without lock runs plain", remote: true, payload: `{"source":"startup"}`, want: [][]string{base}, wantJSON: true},
		{name: "remote with lock runs locked, offline first", remote: true, lock: true, payload: `{"source":"startup"}`,
			want: [][]string{append(append([]string{}, base...), "--locked", "--offline")}, wantJSON: true},
		{name: "mode plain ignores the lock", remote: true, lock: true, session: &config.SessionStartConfig{Mode: "plain"}, payload: `{}`,
			want: [][]string{base}, wantJSON: true},
		{name: "mode frozen", remote: true, session: &config.SessionStartConfig{Mode: "frozen"}, payload: `{}`,
			want: [][]string{append(append([]string{}, base...), "--frozen")}, wantJSON: true},
		{name: "mode locked without a lock still asks for it", remote: true, session: &config.SessionStartConfig{Mode: "locked"}, payload: `{}`,
			want: [][]string{append(append([]string{}, base...), "--locked", "--offline")}, wantJSON: true},
		{name: "mode override beats config", remote: true, session: &config.SessionStartConfig{Mode: "plain"}, ov: Overrides{Mode: "frozen"}, payload: `{}`,
			want: [][]string{append(append([]string{}, base...), "--frozen")}, wantJSON: true},
		{name: "resume generates", remote: true, payload: `{"source":"resume"}`, want: [][]string{base}, wantJSON: true},
		{name: "clear only injects the context", remote: true, payload: `{"source":"clear"}`, wantJSON: true},
		{name: "compact only injects the context", remote: true, payload: `{"source":"compact"}`, wantJSON: true},
		{name: "when always runs locally", session: &config.SessionStartConfig{When: "always"}, payload: `{}`, want: [][]string{base}, wantJSON: true},
		{name: "when override always", ov: Overrides{When: "always"}, payload: `{}`, want: [][]string{base}, wantJSON: true},
		{name: "when always with a local no-op source still injects", session: &config.SessionStartConfig{When: "always"}, payload: `{"source":"compact"}`, wantJSON: true},
		{name: "generate = false is a no-op", remote: true, session: &config.SessionStartConfig{Generate: off()}, payload: `{}`},
		{name: "prompt = false generates silently", remote: true, session: &config.SessionStartConfig{Prompt: off()}, payload: `{}`, want: [][]string{base}},
		{name: "empty payload", remote: true, payload: ``, want: [][]string{base}, wantJSON: true},
		{name: "invalid payload", remote: true, payload: `{not json`, want: [][]string{base}, wantJSON: true},
		{name: "payload of the wrong shape", remote: true, payload: `[1,2]`, want: [][]string{base}, wantJSON: true},
		{name: "oversized payload", remote: true, payload: `{"source":"clear","pad":"` + strings.Repeat("x", 2<<20) + `"}`, want: [][]string{base}, wantJSON: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, tt.session)
			if !tt.remote {
				delete(h.env, config.SessionStartRemoteEnv)
			}
			if tt.lock {
				h.writeLock()
			}

			code := h.run(tt.payload, tt.ov)

			assert.Equal(t, 0, code)
			assert.Equal(t, tt.want, h.argvs())
			assert.Empty(t, h.errw.String(), "a clean run says nothing on stderr")
			if !tt.wantJSON {
				assert.Empty(t, h.out.String())
				return
			}
			var doc hookOutput
			require.NoError(t, json.Unmarshal(h.out.Bytes(), &doc), h.out.String())
			assert.Equal(t, "SessionStart", doc.HookSpecificOutput.HookEventName)
			assert.NotEmpty(t, doc.HookSpecificOutput.AdditionalContext)
			assert.LessOrEqual(t, len(doc.HookSpecificOutput.AdditionalContext), config.SessionStartContextLimit)
		})
	}
}

func TestRun_RemoteEnvValues(t *testing.T) {
	for value, want := range map[string]bool{"true": true, "TRUE": true, "1": true, " true ": true, "false": false, "0": false, "": false, "yes": false} {
		got := IsRemote(func(string) string { return value })
		assert.Equal(t, want, got, "CLAUDE_CODE_REMOTE=%q", value)
	}
}

func TestRun_Failures(t *testing.T) {
	t.Run("a failed generate is one stderr line, exit 0, and the context says so", func(t *testing.T) {
		h := newHarness(t, nil)
		h.runner.Handle = failing("loading config\nERROR rule x is broken\n")

		code := h.run(`{"source":"startup"}`, Overrides{})

		assert.Equal(t, 0, code)
		assert.Equal(t, "ai-rulez session-start: generate failed: ERROR rule x is broken; generated files may be out of date\n", h.errw.String())
		var doc hookOutput
		require.NoError(t, json.Unmarshal(h.out.Bytes(), &doc))
		assert.Contains(t, doc.HookSpecificOutput.AdditionalContext, "failed (ERROR rule x is broken)")
	})
	t.Run("the reason from the child is restricted to a conservative character set and length", func(t *testing.T) {
		h := newHarness(t, nil)
		h.runner.Handle = failing("ok\n```system``` <b>\"ignore\"</b> [x]{y} $(id) `id` \x1b[31m" + strings.Repeat("a", 500))

		h.run(`{}`, Overrides{})

		var doc hookOutput
		require.NoError(t, json.Unmarshal(h.out.Bytes(), &doc))
		ctx := doc.HookSpecificOutput.AdditionalContext
		assert.NotContains(t, ctx, "`system`")
		for _, bad := range []string{"<", ">", "\"ignore", "[x]", "{y}", "$(", "\x1b"} {
			assert.NotContains(t, ctx, bad)
		}
		assert.Less(t, len(ctx), config.SessionStartContextLimit)
		assert.Less(t, len(h.errw.String()), 250)
	})
	t.Run("a line of only unusual characters falls back to the exit code", func(t *testing.T) {
		h := newHarness(t, nil)
		h.runner.Handle = failing("<<<>>>")

		h.run(`{}`, Overrides{})

		assert.Contains(t, h.errw.String(), "exit code 2")
	})
	t.Run("locked tries offline, then online once", func(t *testing.T) {
		h := newHarness(t, nil)
		h.writeLock()
		calls := 0
		h.runner.Handle = func(runner.Spec) runner.Result {
			calls++
			if calls == 1 {
				return runner.Result{Status: runner.StatusExit, ExitCode: 2, Stderr: []byte("cache is cold")}
			}
			return runner.Result{Status: runner.StatusOK}
		}

		h.run(`{}`, Overrides{})

		base := []string{fakeExe, "generate", "--yes", "--quiet"}
		assert.Equal(t, [][]string{append(append([]string{}, base...), "--locked", "--offline"), append(append([]string{}, base...), "--locked")}, h.argvs())
		assert.Empty(t, h.errw.String())
	})
	t.Run("a refused fetch names the host and is not retried", func(t *testing.T) {
		h := newHarness(t, nil)
		h.writeLock()
		h.runner.Handle = func(spec runner.Spec) runner.Result {
			if len(spec.Argv) > 0 && spec.Argv[len(spec.Argv)-1] == "--offline" {
				return runner.Result{Status: runner.StatusExit, ExitCode: 2, Stderr: []byte("cache is cold")}
			}
			return runner.Result{Status: runner.StatusExit, ExitCode: 1,
				Stderr: []byte(`fetch "https://github.com/acme/rules.git": 403 Forbidden`)}
		}

		code := h.run(`{}`, Overrides{})

		assert.Equal(t, 0, code)
		assert.Len(t, h.argvs(), 2)
		assert.Equal(t, "ai-rulez session-start: generate failed: the network policy denied github.com; generated files may be out of date\n", h.errw.String())
	})
	t.Run("a refusal without a host still reads as one line", func(t *testing.T) {
		h := newHarness(t, nil)
		h.runner.Handle = failing("proxy said 407 Proxy Authentication Required")

		h.run(`{}`, Overrides{})

		assert.Equal(t, "ai-rulez session-start: generate failed: the network policy denied the request; generated files may be out of date\n", h.errw.String())
	})
	t.Run("a timeout is reported and not retried", func(t *testing.T) {
		h := newHarness(t, nil)
		h.writeLock()
		h.runner.Handle = func(runner.Spec) runner.Result {
			return runner.Result{Status: runner.StatusTimeout, ExitCode: -1, Timeout: 90 * time.Second}
		}

		code := h.run(`{}`, Overrides{})

		assert.Equal(t, 0, code)
		assert.Len(t, h.argvs(), 1)
		assert.Equal(t, "ai-rulez session-start: generate failed: timed out after 1m30s; generated files may be out of date\n", h.errw.String())
	})
	t.Run("the binary cannot start", func(t *testing.T) {
		h := newHarness(t, nil)
		h.runner.Handle = func(runner.Spec) runner.Result { return runner.Result{Status: runner.StatusUnavailable} }

		assert.Equal(t, 0, h.run(`{}`, Overrides{}))
		assert.Contains(t, h.errw.String(), "the ai-rulez binary could not be started")
	})
	t.Run("the whole run is bounded by the timeout", func(t *testing.T) {
		h := newHarness(t, nil)
		var seen time.Duration
		h.runner.Handle = func(spec runner.Spec) runner.Result {
			seen = spec.Timeout
			return runner.Result{Status: runner.StatusOK}
		}

		h.run(`{}`, Overrides{Timeout: 7 * time.Second})

		assert.InDelta(t, float64(7*time.Second), float64(seen), float64(time.Second))
	})
	t.Run("the configured timeout is the default bound", func(t *testing.T) {
		h := newHarness(t, &config.SessionStartConfig{Timeout: 30})
		var seen time.Duration
		h.runner.Handle = func(spec runner.Spec) runner.Result {
			seen = spec.Timeout
			return runner.Result{Status: runner.StatusOK}
		}

		h.run(`{}`, Overrides{})

		assert.InDelta(t, float64(30*time.Second), float64(seen), float64(time.Second))
	})
	t.Run("long output is cut to one bounded line", func(t *testing.T) {
		h := newHarness(t, nil)
		h.runner.Handle = failing(strings.Repeat("é", 5000))

		h.run(`{}`, Overrides{})

		assert.Equal(t, 1, strings.Count(h.errw.String(), "\n"))
		assert.Less(t, len(h.errw.String()), 500)
	})
}

func TestRun_Environment(t *testing.T) {
	h := newHarness(t, nil)
	h.env["ANTHROPIC_API_KEY"] = "sk-secret"
	h.env["GITHUB_TOKEN"] = "ghp_secret"
	h.env["HTTPS_PROXY"] = "http://proxy:3128"
	h.env["AI_RULEZ_PROFILE"] = "backend"
	h.env["AI_RULEZ_GIT_TOKEN"] = "tok"
	h.env["NODE_EXTRA_CA_CERTS"] = "/etc/ca.pem"

	h.run(`{}`, Overrides{})

	calls := h.runner.Calls()
	require.Len(t, calls, 1)
	env := strings.Join(calls[0].Env, "\n")
	assert.Contains(t, env, "HTTPS_PROXY=http://proxy:3128")
	assert.Contains(t, env, "NODE_EXTRA_CA_CERTS=/etc/ca.pem")
	assert.Contains(t, env, "AI_RULEZ_PROFILE=backend")
	assert.Contains(t, env, "AI_RULEZ_ACK_COMMANDS=1")
	assert.NotContains(t, env, "sk-secret")
	assert.NotContains(t, env, "ghp_secret")
	assert.NotContains(t, env, "AI_RULEZ_GIT_TOKEN", "credential-shaped AI_RULEZ variables are not passed on")
	assert.Equal(t, h.root, calls[0].Dir)
	assert.Nil(t, calls[0].Stdin, "the child does not read the hook payload")
}

func TestRun_StdoutIsOnlyTheHookDocument(t *testing.T) {
	h := newHarness(t, nil)
	h.runner.Handle = func(runner.Spec) runner.Result {
		return runner.Result{Status: runner.StatusOK, Stdout: []byte("WARN something\n"), Stderr: []byte("WARN other\n")}
	}

	h.run(`{}`, Overrides{})

	var doc map[string]any
	require.NoError(t, json.Unmarshal(h.out.Bytes(), &doc), h.out.String())
	assert.Contains(t, doc, "hookSpecificOutput")
	assert.Equal(t, 1, strings.Count(h.out.String(), "\n"))
	assert.Empty(t, h.errw.String(), "child output is not forwarded when generate succeeds")
}

func TestRun_ProjectDirectory(t *testing.T) {
	t.Run("no configuration is a silent no-op", func(t *testing.T) {
		h := newHarness(t, nil)
		require.NoError(t, os.RemoveAll(filepath.Join(h.root, ".ai-rulez")))

		assert.Equal(t, 0, h.run(`{}`, Overrides{}))

		assert.Empty(t, h.out.String())
		assert.Empty(t, h.errw.String())
		assert.Empty(t, h.argvs())
	})
	t.Run("outside a git work tree is one line", func(t *testing.T) {
		h := newHarness(t, nil)
		require.NoError(t, os.RemoveAll(filepath.Join(h.root, ".git")))
		h.deps.Getwd = func() (string, error) { return h.root, nil }

		assert.Equal(t, 0, h.run(`{}`, Overrides{}))

		assert.Empty(t, h.argvs())
		assert.Equal(t, "ai-rulez session-start: "+h.root+" is not in a git work tree; skipped\n", h.errw.String())
	})
	t.Run("a configuration that does not load is one line", func(t *testing.T) {
		h := newHarness(t, nil)
		h.deps.LoadConfig = func(context.Context, string) (*config.Config, error) { return nil, os.ErrInvalid }

		assert.Equal(t, 0, h.run(`{}`, Overrides{}))

		assert.Empty(t, h.argvs())
		assert.Equal(t, "ai-rulez session-start: cannot load the configuration; skipped\n", h.errw.String())
	})
	t.Run("CLAUDE_PROJECT_DIR wins over the payload cwd and the working directory", func(t *testing.T) {
		h := newHarness(t, nil)
		sub := filepath.Join(h.root, "pkg", "deep")
		require.NoError(t, os.MkdirAll(sub, 0o755))
		h.env["CLAUDE_PROJECT_DIR"] = sub
		h.deps.Getwd = func() (string, error) { return "/nonexistent", nil }
		var loaded string
		h.deps.LoadConfig = func(_ context.Context, dir string) (*config.Config, error) { loaded = dir; return h.cfg, nil }

		h.run(`{"cwd":"/elsewhere"}`, Overrides{})

		assert.Equal(t, h.root, loaded, "the configuration is found by walking up from the project directory")
		require.Len(t, h.runner.Calls(), 1)
		assert.Equal(t, h.root, h.runner.Calls()[0].Dir)
	})
	t.Run("the payload cwd is used without CLAUDE_PROJECT_DIR", func(t *testing.T) {
		h := newHarness(t, nil)
		h.deps.Getwd = func() (string, error) { return "/nonexistent", nil }

		h.run(`{"cwd":`+quoteJSON(h.root)+`}`, Overrides{})

		assert.Len(t, h.runner.Calls(), 1)
	})
	t.Run("a relative payload cwd is ignored", func(t *testing.T) {
		h := newHarness(t, nil)

		h.run(`{"cwd":"../../etc"}`, Overrides{})

		require.Len(t, h.runner.Calls(), 1)
		assert.Equal(t, h.root, h.runner.Calls()[0].Dir, "falls back to the working directory")
	})
	t.Run("a read-only repository still runs: nothing is written into it by the hook", func(t *testing.T) {
		if runtime.GOOS == "windows" || os.Geteuid() == 0 {
			t.Skip("directory permission bits do not stop writes on windows or for root, so the case would assert nothing")
		}
		h := newHarness(t, nil)
		require.NoError(t, os.Chmod(h.root, 0o555))
		t.Cleanup(func() { _ = os.Chmod(h.root, 0o755) })

		assert.Equal(t, 0, h.run(`{}`, Overrides{}))

		assert.Len(t, h.runner.Calls(), 1)
	})
	t.Run("a dirty tree is not a reason to skip", func(t *testing.T) {
		h := newHarness(t, nil)
		require.NoError(t, os.WriteFile(filepath.Join(h.root, "uncommitted.txt"), []byte("x"), 0o644))

		h.run(`{}`, Overrides{})

		assert.Len(t, h.runner.Calls(), 1)
	})
	t.Run("a repository without .git/config still counts as a work tree", func(t *testing.T) {
		h := newHarness(t, nil)
		require.NoFileExists(t, filepath.Join(h.root, ".git", "config"))

		h.run(`{}`, Overrides{})

		assert.Len(t, h.runner.Calls(), 1)
	})
	t.Run("a .git file (worktree or submodule) counts", func(t *testing.T) {
		h := newHarness(t, nil)
		require.NoError(t, os.RemoveAll(filepath.Join(h.root, ".git")))
		require.NoError(t, os.WriteFile(filepath.Join(h.root, ".git"), []byte("gitdir: /elsewhere\n"), 0o644))

		h.run(`{}`, Overrides{})

		assert.Len(t, h.runner.Calls(), 1)
	})
}

func quoteJSON(s string) string {
	data, _ := json.Marshal(s)
	return string(data)
}

func TestRun_ConcurrentRunsAreSerialized(t *testing.T) {
	h := newHarness(t, nil)
	var active, peak, calls atomic.Int32
	h.runner.Handle = func(runner.Spec) runner.Result {
		n := active.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		calls.Add(1)
		time.Sleep(150 * time.Millisecond)
		active.Add(-1)
		return runner.Result{Status: runner.StatusOK}
	}
	var wg sync.WaitGroup
	for range 3 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var out, errw bytes.Buffer
			assert.Equal(t, 0, Run(context.Background(), strings.NewReader(`{"source":"startup"}`), &out, &errw, h.deps, Overrides{}))
			assert.Empty(t, errw.String())
		}()
	}
	wg.Wait()

	assert.Equal(t, int32(1), peak.Load(), "only one generate runs at a time")
	assert.Equal(t, int32(3), calls.Load(), "each run still regenerates once the lock is free")
	entries, err := os.ReadDir(filepath.Dir(lockPath(h.deps, h.root)))
	require.NoError(t, err)
	assert.Empty(t, entries, "the lock file is released")
}

func TestRun_LockFile(t *testing.T) {
	t.Run("a stale lock is taken over", func(t *testing.T) {
		h := newHarness(t, &config.SessionStartConfig{Timeout: 5})
		path := lockPath(h.deps, h.root)
		require.NoError(t, os.WriteFile(path, []byte("123\n"), 0o600))
		old := time.Now().Add(-time.Hour)
		require.NoError(t, os.Chtimes(path, old, old))

		h.run(`{}`, Overrides{})

		assert.Len(t, h.runner.Calls(), 1)
		assert.Empty(t, h.errw.String())
	})
	t.Run("a live lock that outlasts the timeout skips generate with one line", func(t *testing.T) {
		h := newHarness(t, nil)
		require.NoError(t, os.WriteFile(lockPath(h.deps, h.root), []byte("123\n"), 0o600))

		code := h.run(`{}`, Overrides{Timeout: 300 * time.Millisecond})

		assert.Equal(t, 0, code)
		assert.Empty(t, h.runner.Calls())
		assert.Equal(t, "ai-rulez session-start: another session-start run did not finish in time\n", h.errw.String())
		assert.Contains(t, h.out.String(), "additionalContext", "the context still reaches the agent")
	})
	t.Run("the lock lives in a private per-user directory", func(t *testing.T) {
		h := newHarness(t, nil)
		h.deps.UID = os.Getuid()

		path := lockPath(h.deps, h.root)

		dir := filepath.Dir(path)
		assert.Equal(t, h.deps.TempDir, filepath.Dir(dir))
		assert.Contains(t, filepath.Base(dir), "ai-rulez-session-start-")
		info, err := os.Lstat(dir)
		require.NoError(t, err)
		assert.True(t, info.IsDir())
		if runtime.GOOS != "windows" {
			assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
		}
	})
	t.Run("a looser directory of ours is tightened", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("POSIX permission bits")
		}
		h := newHarness(t, nil)
		h.deps.UID = os.Getuid()
		dir := filepath.Dir(lockPath(h.deps, h.root))
		require.NoError(t, os.Chmod(dir, 0o777)) //nolint:gosec // a deliberately loose directory

		path := lockPath(h.deps, h.root)

		assert.NotEmpty(t, path)
		info, err := os.Stat(dir)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
	})
	t.Run("a symlink in place of the directory runs unlocked", func(t *testing.T) {
		h := newHarness(t, nil)
		h.deps.UID = os.Getuid()
		target := t.TempDir()
		itestutil.SymlinkOrSkip(t, target, filepath.Join(h.deps.TempDir, fmt.Sprintf("ai-rulez-session-start-%d", h.deps.UID)))

		assert.Empty(t, lockPath(h.deps, h.root))
		h.run(`{}`, Overrides{})
		assert.Len(t, h.runner.Calls(), 1)
		entries, err := os.ReadDir(target)
		require.NoError(t, err)
		assert.Empty(t, entries, "nothing was written through the link")
	})
	t.Run("an unusable temp directory runs unlocked", func(t *testing.T) {
		h := newHarness(t, nil)
		h.deps.TempDir = filepath.Join(h.root, "does", "not", "exist")

		h.run(`{}`, Overrides{})

		assert.Len(t, h.runner.Calls(), 1)
	})
}

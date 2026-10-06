package skillsearch

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/llm"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuild_IsIncrementalAndDigestKeyed(t *testing.T) {
	t.Parallel()
	// Arrange
	items := refundCatalog()
	emb := refundEmbedder()
	first, err := Build(t.Context(), items, &BuildOptions{Embedder: emb})
	require.NoError(t, err)
	require.Equal(t, 4, first.Embedded)
	emb.texts = nil

	// Act: edit one description, change another skill's digest only (its body)
	items[0].Doc.Description = "Reimburse a customer, including partial credit"
	items[1].Digest = "sha256:edited-body"
	second, err := Build(t.Context(), items, &BuildOptions{Embedder: emb, Old: first.Index})

	// Assert: one text embedded, three reused, the digest-only change costs nothing
	require.NoError(t, err)
	assert.Equal(t, 1, second.Embedded)
	assert.Equal(t, 3, second.Reused)
	require.Len(t, emb.texts, 1)
	assert.Contains(t, emb.texts[0], "partial credit")
	row := second.Index.Row("", "deploy-staging")
	assert.Equal(t, "sha256:edited-body", second.Index.Manifest.Items[row].ItemDigest, "the digest is refreshed without re-embedding")
}

func TestBuild_NothingChangedMakesNoCalls(t *testing.T) {
	t.Parallel()
	items := refundCatalog()
	emb := refundEmbedder()
	first, err := Build(t.Context(), items, &BuildOptions{Embedder: emb})
	require.NoError(t, err)
	emb.calls = 0

	again, err := Build(t.Context(), items, &BuildOptions{Embedder: emb, Old: first.Index})

	require.NoError(t, err)
	assert.Zero(t, emb.calls)
	m1, v1, _ := first.Index.Bytes()
	m2, v2, _ := again.Index.Bytes()
	assert.Equal(t, m1, m2)
	assert.Equal(t, v1, v2, "an unchanged rebuild is byte-identical")
}

func TestBuild_RenameReusesTheVector(t *testing.T) {
	t.Parallel()
	// the embedded text of a renamed skill contains its new name, so it re-embeds: only a skill
	// whose embedded fields exclude the name keeps its vector across a rename
	items := refundCatalog()
	emb := refundEmbedder()
	cfg := Config{Fields: []string{FieldDescription}}
	first, err := Build(t.Context(), items, &BuildOptions{Config: cfg, Embedder: emb})
	require.NoError(t, err)
	emb.calls = 0
	items[0].ID, items[0].Doc.Name = "refund-customer", "refund-customer"

	again, err := Build(t.Context(), items, &BuildOptions{Config: cfg, Embedder: emb, Old: first.Index})

	require.NoError(t, err)
	assert.Zero(t, emb.calls, "same text under a new id: one lookup by text digest")
	assert.GreaterOrEqual(t, again.Index.Row("", "refund-customer"), 0)
	assert.Equal(t, -1, again.Index.Row("", "issue-refund"), "the old row is dropped")
}

func TestBuild_InvalidationMatrix(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(o *BuildOptions, e *conceptEmbedder)
	}{
		{"model", func(_ *BuildOptions, e *conceptEmbedder) { e.name = "concepts-v2" }},
		{"provider", func(_ *BuildOptions, e *conceptEmbedder) { e.fp = "other@host" }},
		{"fields", func(o *BuildOptions, _ *conceptEmbedder) { o.Config.Fields = []string{FieldName} }},
		{"body", func(o *BuildOptions, _ *conceptEmbedder) { o.Config.IndexBody = true }},
		{"rebuild flag", func(o *BuildOptions, _ *conceptEmbedder) { o.Rebuild = true }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			items := refundCatalog()
			emb := refundEmbedder()
			first, err := Build(t.Context(), items, &BuildOptions{Embedder: emb})
			require.NoError(t, err)
			emb.calls = 0
			opts := &BuildOptions{Embedder: emb, Old: first.Index}
			tt.mutate(opts, emb)

			// Act
			again, err := Build(t.Context(), items, opts)

			// Assert: nothing is mixed in: every vector is re-embedded
			require.NoError(t, err)
			assert.Equal(t, 4, again.Embedded, "reused %d", again.Reused)
			assert.Zero(t, again.Reused)
		})
	}
}

func TestBuild_OnlyReembedsTheNamedItems(t *testing.T) {
	t.Parallel()
	items := refundCatalog()
	emb := refundEmbedder()
	first, err := Build(t.Context(), items, &BuildOptions{Embedder: emb})
	require.NoError(t, err)
	emb.calls = 0

	again, err := Build(t.Context(), items, &BuildOptions{Embedder: emb, Old: first.Index, Only: map[string]bool{"git-workflow": true}})

	require.NoError(t, err)
	assert.Equal(t, 1, again.Embedded)
	assert.Equal(t, 3, again.Reused)
}

func TestBuild_WithholdsSecrets(t *testing.T) {
	t.Parallel()
	// Arrange
	items := refundCatalog()
	items[1].Doc.Description = "Deploy with the key AKIAABCDEFGHIJKLMNOP"
	scan := func(text string) (string, bool) {
		if strings.Contains(text, "AKIA") {
			return "aws-access-key", true
		}
		return "", false
	}
	emb := refundEmbedder()

	// Act
	res, err := Build(t.Context(), items, &BuildOptions{Embedder: emb, Scanner: scan})

	// Assert: the text never reached the embedder
	require.NoError(t, err)
	assert.Equal(t, []Withheld{{ID: "deploy-staging", Kind: "aws-access-key"}}, res.Plan.Withheld)
	assert.Equal(t, 3, res.Embedded)
	assert.Equal(t, []string{"deploy-staging"}, res.Missing)
	for _, sent := range emb.texts {
		assert.NotContains(t, sent, "AKIA")
	}
}

func TestBuild_BudgetStopKeepsFinishedBatches(t *testing.T) {
	t.Parallel()
	// Arrange: the second batch fails with the budget error
	items := refundCatalog()
	emb := &failAfter{conceptEmbedder: refundEmbedder(), okCalls: 1, err: llm.ErrBudget}

	// Act
	res, err := Build(t.Context(), items, &BuildOptions{Embedder: emb, BatchSize: 2})

	// Assert
	require.NoError(t, err)
	require.ErrorIs(t, res.Err, llm.ErrBudget)
	assert.Equal(t, 2, res.Embedded)
	require.NotNil(t, res.Index)
	assert.Equal(t, 2, res.Index.Len())
	assert.Len(t, res.Missing, 2)
}

type failAfter struct {
	*conceptEmbedder
	okCalls int
	err     error
}

func (f *failAfter) Embed(ctx context.Context, texts []string) (Embedding, error) {
	if f.okCalls <= 0 {
		return Embedding{}, f.err
	}
	f.okCalls--
	return f.conceptEmbedder.Embed(ctx, texts)
}

func TestBuild_BudgetStopBeforeAnyVectorIsNotAnError(t *testing.T) {
	t.Parallel()
	emb := &failAfter{conceptEmbedder: refundEmbedder(), err: llm.ErrBudget}

	res, err := Build(t.Context(), refundCatalog(), &BuildOptions{Embedder: emb})

	require.NoError(t, err)
	assert.ErrorIs(t, res.Err, llm.ErrBudget)
	assert.Nil(t, res.Index)
}

type wrongDims struct{ *conceptEmbedder }

func (w *wrongDims) Embed(ctx context.Context, texts []string) (Embedding, error) {
	e, err := w.conceptEmbedder.Embed(ctx, texts)
	if len(e.Vectors) > 1 {
		e.Vectors[1] = e.Vectors[1][:2]
	}
	return e, err
}

func TestBuild_RejectsMixedDimensions(t *testing.T) {
	t.Parallel()
	_, err := Build(t.Context(), refundCatalog(), &BuildOptions{Embedder: &wrongDims{refundEmbedder()}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dimensions")
}

func TestBuild_Float16(t *testing.T) {
	t.Parallel()
	items := refundCatalog()
	res, err := Build(t.Context(), items, &BuildOptions{Config: Config{DType: DTypeFloat16}, Embedder: refundEmbedder()})
	require.NoError(t, err)
	assert.Equal(t, DTypeFloat16, res.Index.Manifest.DType)
	dir := t.TempDir()
	require.NoError(t, WriteIndex(dir, res.Index))
	loaded, err := LoadIndex(dir)
	require.NoError(t, err)
	r := &Ranker{Items: items, Cfg: Config{Mode: ModeHybrid, DType: DTypeFloat16}, Index: loaded, Embedder: refundEmbedder()}
	assert.Equal(t, "issue-refund", names(r, r.Search(t.Context(), "money back").Hits)[0])
}

func TestPlanBuild_CountsWithoutCalling(t *testing.T) {
	t.Parallel()
	items := refundCatalog()
	emb := refundEmbedder()
	first, err := Build(t.Context(), items, &BuildOptions{Embedder: emb})
	require.NoError(t, err)
	emb.calls = 0
	items[2].Doc.Description = "changed"

	plan := PlanBuild(items, &BuildOptions{Embedder: emb, Old: first.Index})

	assert.Equal(t, 4, plan.Total)
	assert.Equal(t, 3, plan.Reused)
	assert.Equal(t, 1, plan.ToEmbed)
	assert.Positive(t, plan.Bytes)
	assert.Positive(t, plan.EstTokens)
	assert.Zero(t, emb.calls)

	emb.name = "other-model"
	assert.Contains(t, PlanBuild(items, &BuildOptions{Embedder: emb, Old: first.Index}).Reason, "model changed")
}

func TestLock_SecondRunIsRefused(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	release, err := Lock(dir, nil)
	require.NoError(t, err)

	_, err = Lock(dir, nil)
	require.ErrorIs(t, err, ErrLocked)

	release()
	release2, err := Lock(dir, nil)
	require.NoError(t, err)
	release2()
}

func TestLock_StaleLockIsTakenOver(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	p := filepath.Join(dir, lockFile)
	require.NoError(t, os.WriteFile(p, []byte("1\n"), 0o600))
	old := time.Now().Add(-time.Hour)
	require.NoError(t, os.Chtimes(p, old, old))

	release, err := Lock(dir, nil)

	require.NoError(t, err)
	release()
}

func TestWriteIndex_AtomicOnFailure(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("directory permissions differ")
	}
	// a manifest write that cannot happen leaves the previous index intact
	items := refundCatalog()
	res, err := Build(t.Context(), items, &BuildOptions{Embedder: refundEmbedder()})
	require.NoError(t, err)
	dir := t.TempDir()
	require.NoError(t, WriteIndex(dir, res.Index))
	before, err := os.ReadFile(filepath.Join(dir, ManifestFile))
	require.NoError(t, err)
	// a directory named like the temp target makes the rename of the manifest fail
	require.NoError(t, os.Remove(filepath.Join(dir, ManifestFile)))
	require.NoError(t, os.Mkdir(filepath.Join(dir, ManifestFile), 0o750))

	err = WriteIndex(dir, res.Index)

	require.Error(t, err)
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		assert.NotContains(t, e.Name(), ".tmp-", "no temp file is left behind")
	}
	assert.NotEmpty(t, before)
}

func TestCommandEmbedder(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script")
	}
	script := func(t *testing.T, body string) string {
		p := filepath.Join(t.TempDir(), "embed.sh")
		require.NoError(t, os.WriteFile(p, []byte("#!/bin/sh\n"+body), 0o700))
		return p
	}
	tests := []struct {
		name    string
		body    string
		env     map[string]string
		wantErr string
		check   func(t *testing.T, e Embedding)
	}{
		{"vectors", `cat >/dev/null; echo '{"vectors":[[1,0],[0,1]]}'`, nil, "", func(t *testing.T, e Embedding) {
			assert.Equal(t, [][]float32{{1, 0}, {0, 1}}, e.Vectors)
		}},
		{"wrong count", `cat >/dev/null; echo '{"vectors":[[1,0]]}'`, nil, "1 vectors for 2 texts", nil},
		{"not json", `cat >/dev/null; echo nope`, nil, "did not print", nil},
		{"fails", `echo boom >&2; exit 3`, nil, "boom", nil},
		{"input arrives as json", `grep -q '"input":\["a","b"\]' || exit 9; echo '{"vectors":[[1],[2]]}'`, nil, "", nil},
		{"only listed env is passed", `cat >/dev/null; test -z "$SECRET_NOT_PASSED" && test "$PASSED" = yes || exit 8; echo '{"vectors":[[1],[2]]}'`,
			map[string]string{"SECRET_NOT_PASSED": "x", "PASSED": "yes"}, "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			e := &CommandEmbedder{
				Argv: []string{script(t, tt.body)}, PassEnv: []string{"PASSED"},
				Getenv: func(k string) string {
					if k == "PATH" {
						return "/usr/bin:/bin"
					}
					if k == "SECRET_NOT_PASSED" && tt.env == nil {
						return ""
					}
					return tt.env[k]
				},
			}
			e.Getenv = func(k string) string {
				if k == "PATH" {
					return "/usr/bin:/bin"
				}
				if k == "SECRET_NOT_PASSED" {
					return "" // the scrubbed environment never looks it up
				}
				return tt.env[k]
			}

			// Act
			got, err := e.Embed(t.Context(), []string{"a", "b"})

			// Assert
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

func TestCommandEmbedder_StdoutCapAndFingerprint(t *testing.T) {
	t.Parallel()
	capped := &runner.Fake{Handle: func(runner.Spec) runner.Result {
		return runner.Result{Status: runner.StatusOK, Stdout: []byte(`{"vectors":[[1]]}`), StdoutTruncated: true}
	}}
	_, err := (&CommandEmbedder{Argv: []string{"embed"}, Runner: capped}).Embed(t.Context(), []string{"a"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "wrote more than")
	e := &CommandEmbedder{Argv: []string{"/opt/embed", "--secret-flag"}, ModelName: "m"}
	assert.Equal(t, "command:/opt/embed", e.Fingerprint(), "arguments never enter the fingerprint")
	assert.Equal(t, "m", e.Model())
	_, err = (&CommandEmbedder{}).Embed(t.Context(), []string{"a"})
	assert.Error(t, err)
}

func TestLLMEmbedder_UsesTheManagedClient(t *testing.T) {
	t.Parallel()
	fake := llm.NewFake()
	e := &LLMEmbedder{Client: fake, ModelName: "emb", Provider: "fake@local"}

	got, err := e.Embed(t.Context(), []string{"one", "two"})

	require.NoError(t, err)
	assert.Len(t, got.Vectors, 2)
	assert.Equal(t, "fake@local", e.Fingerprint())
	require.Len(t, fake.EmbedCalls(), 1)
	assert.Empty(t, fake.EmbedCalls()[0].Model, "the client's own embedding_model routing is not overridden")
	assert.Equal(t, "emb", e.Model())
	e.RequestModel = "override"
	_, err = e.Embed(t.Context(), []string{"x"})
	require.NoError(t, err)
	assert.Equal(t, "override", fake.EmbedCalls()[1].Model)
}

func TestLLMEmbedder_NetworkDisabledDegradesAsNetworkDisabled(t *testing.T) {
	t.Parallel()
	managed, err := llm.New(llm.Config{EmbeddingModel: "emb"}, llm.Options{})
	require.NoError(t, err)
	e := &LLMEmbedder{Client: managed, ModelName: "emb"}

	_, err = e.Embed(t.Context(), []string{"x"})

	require.Error(t, err)
	assert.Equal(t, DegradedNetworkDisabled, DegradedReason(err))
}

func TestConfig_Validate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		cfg  *Config
		want string
	}{
		{"nil", nil, ""},
		{"zero", &Config{}, ""},
		{"full", &Config{Mode: "hybrid", Fusion: "weighted", DType: "float16", Fields: []string{"name"}, IndexDir: "search-index"}, ""},
		{"mode", &Config{Mode: "semantic"}, "search.mode"},
		{"fusion", &Config{Fusion: "sum"}, "search.fusion"},
		{"dtype", &Config{DType: "int8"}, "search.dtype"},
		{"field", &Config{Fields: []string{"body"}}, "search.fields"},
		{"dir escapes", &Config{IndexDir: "../elsewhere"}, "inside the config directory"},
		{"dir absolute", &Config{IndexDir: "/tmp/x"}, "relative path"},
		{"dir dotdot inside", &Config{IndexDir: "a/../../b"}, "inside the config directory"},
		{"timeout", &Config{QueryTimeoutMS: -1}, "query_timeout_ms"},
		{"empty command", &Config{Embeddings: &EmbeddingsConfig{Command: []string{" "}}}, "must start with a program"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := strings.Join(tt.cfg.Validate(), "; ")
			if tt.want == "" {
				assert.Empty(t, got)
			} else {
				assert.Contains(t, got, tt.want)
			}
		})
	}
	assert.True(t, CommittedIndexDir("search-index"))
	assert.False(t, CommittedIndexDir("local/search"))
	assert.False(t, CommittedIndexDir(""))
}

func TestBuild_ConfigBatchSizeSplitsTheCalls(t *testing.T) {
	t.Parallel()
	emb := refundEmbedder()

	res, err := Build(t.Context(), refundCatalog(), &BuildOptions{Config: Config{BatchSize: 1}, Embedder: emb})

	require.NoError(t, err)
	assert.Equal(t, 4, res.Calls, "one text per call")
	assert.Equal(t, 4, res.Embedded)
}

// A grandchild that inherits stdout must not keep the call alive past its timeout, and must
// not outlive the call.
func TestCommandEmbedder_TimeoutKillsTheProcessGroup(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script")
	}
	tests := []struct {
		name    string
		body    string
		timeout time.Duration
		wantErr string
	}{
		{"hung child with a grandchild on stdout", "sleep 30 &\nsleep 30\n", 400 * time.Millisecond, "did not finish"},
		{"child exits while a grandchild keeps stdout open", "cat >/dev/null\nsleep 30 &\necho '{\"vectors\":[[1]]}'\n", 20 * time.Second, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			p := filepath.Join(t.TempDir(), "embed.sh")
			require.NoError(t, os.WriteFile(p, []byte("#!/bin/sh\n"+tt.body), 0o700))
			e := &CommandEmbedder{Argv: []string{p}, Timeout: tt.timeout, Getenv: func(k string) string {
				if k == "PATH" {
					return "/usr/bin:/bin"
				}
				return ""
			}}
			start := ambient.Clock(nil).Now()

			// Act
			_, err := e.Embed(t.Context(), []string{"a"})

			// Assert
			elapsed := ambient.Clock(nil).Now().Sub(start)
			assert.Less(t, elapsed, 6*time.Second, "the timeout must bound the call even with a grandchild holding stdout")
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestCommandEmbedder_PassesTheWindowsBaseEnvironmentWhenSet(t *testing.T) {
	t.Parallel()
	// Arrange
	fake := &runner.Fake{Handle: func(runner.Spec) runner.Result {
		return runner.Result{Status: runner.StatusOK, Stdout: []byte(`{"vectors":[[1]]}`)}
	}}
	vars := map[string]string{"PATH": "p", "HOME": "h", "SYSTEMROOT": `C:\Windows`, "PATHEXT": ".EXE", "TEMP": `C:\t`, "USERPROFILE": `C:\u`, "OTHER": "no"}
	e := &CommandEmbedder{Argv: []string{"embed"}, Runner: fake, Getenv: func(k string) string { return vars[k] }}

	// Act
	_, err := e.Embed(t.Context(), []string{"a"})

	// Assert
	require.NoError(t, err)
	require.Len(t, fake.Calls(), 1)
	assert.ElementsMatch(t, []string{"PATH=p", "HOME=h", `SYSTEMROOT=C:\Windows`, "PATHEXT=.EXE", `TEMP=C:\t`, `USERPROFILE=C:\u`}, fake.Calls()[0].Env)
}

func TestLock_StaleIsJudgedByTheInjectedClock(t *testing.T) {
	t.Parallel()
	// Arrange: a lock last touched at noon, and a clock 11 minutes later
	dir := t.TempDir()
	p := filepath.Join(dir, lockFile)
	require.NoError(t, os.WriteFile(p, []byte("1 1\n"), 0o600))
	noon := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	require.NoError(t, os.Chtimes(p, noon, noon))

	// Act and Assert: fresh under a clock at 12:05, stale at 12:11
	_, err := Lock(dir, ambient.Fixed(noon.Add(5*time.Minute)))
	require.ErrorIs(t, err, ErrLocked)
	release, err := Lock(dir, ambient.Fixed(noon.Add(11*time.Minute)))
	require.NoError(t, err)
	release()
}

func TestLock_HeartbeatKeepsALongRunFromBeingTakenOver(t *testing.T) {
	t.Parallel()
	// Arrange
	dir := t.TempDir()
	p := filepath.Join(dir, lockFile)
	release, err := lockWith(dir, nil, 10*time.Millisecond)
	require.NoError(t, err)
	defer release()
	old := time.Now().Add(-time.Hour)
	require.NoError(t, os.Chtimes(p, old, old))

	// Act and Assert: the held lock refreshes its own modification time
	require.Eventually(t, func() bool {
		info, err := os.Stat(p)
		return err == nil && time.Since(info.ModTime()) < time.Minute
	}, 5*time.Second, 10*time.Millisecond)
	_, err = Lock(dir, nil)
	require.ErrorIs(t, err, ErrLocked)
}

func TestLock_ReleaseLeavesAnotherRunsLockAlone(t *testing.T) {
	t.Parallel()
	// Arrange: our lock was taken over by another run
	dir := t.TempDir()
	p := filepath.Join(dir, lockFile)
	release, err := Lock(dir, nil)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(p, []byte("99999 7\n"), 0o600))

	// Act
	release()
	release() // idempotent

	// Assert
	got, err := os.ReadFile(p)
	require.NoError(t, err)
	assert.Equal(t, "99999 7\n", string(got))
}

package llm

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

// Live QA of the backends beyond the parity checks in live_test.go: cache
// behaviour on real replies, budget stops, typed errors. Gated by
// AI_RULEZ_LIVE_LLM=1 and GEMINI_API_KEY; every request is a few tokens.

// recorder wraps a Client and remembers what was sent and what came back.
type recorder struct {
	Client
	mu    sync.Mutex
	reqs  []ChatRequest
	resps []ChatResponse
	embed []EmbedResponse
}

func (r *recorder) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	resp, err := r.Client.Chat(ctx, req)
	r.mu.Lock()
	r.reqs = append(r.reqs, req)
	if err == nil {
		r.resps = append(r.resps, resp)
	}
	r.mu.Unlock()
	return resp, err
}

func (r *recorder) Embed(ctx context.Context, req EmbedRequest) (EmbedResponse, error) {
	resp, err := r.Client.Embed(ctx, req)
	r.mu.Lock()
	if err == nil {
		r.embed = append(r.embed, resp)
	}
	r.mu.Unlock()
	return resp, err
}

func (r *recorder) usage() Usage {
	r.mu.Lock()
	defer r.mu.Unlock()
	var u Usage
	for _, x := range r.resps {
		if !x.Cached {
			u.PromptTokens += x.Usage.PromptTokens
			u.CompletionTokens += x.Usage.CompletionTokens
		}
	}
	for _, x := range r.embed {
		if !x.Cached {
			u.PromptTokens += x.Usage.PromptTokens
		}
	}
	return u
}

// cachedLiveClient builds a live client with the cache on, rooted in a temp dir.
func cachedLiveClient(t *testing.T, backend string) (*Managed, Options) {
	t.Helper()
	cfg := liveConfig(backend)
	cfg.Cache = ptr(true)
	cfg.MaxCalls = 1000 // an active budget makes Spent() count provider calls
	dir := t.TempDir()
	opts := Options{CacheDir: filepath.Join(dir, "cache"), SecretPath: filepath.Join(dir, "cfg", "llm-cache.key"), Getenv: os.Getenv}
	m, err := New(cfg, opts)
	if err != nil {
		t.Fatalf("%s: %v", backend, err)
	}
	t.Cleanup(func() { _ = m.Close() })
	return m, opts
}

func cacheFiles(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	_ = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err == nil && info.Mode().IsRegular() && strings.HasSuffix(p, ".json") {
			out = append(out, p)
		}
		return nil
	})
	return out
}

func tinyChat(prompt string) ChatRequest {
	return ChatRequest{Messages: []Message{{Role: RoleUser, Content: prompt}}, MaxTokens: 32, PromptVersion: "qa/v1"}
}

func TestLiveCacheHitsOnRepeatAndMissesOnVersionChange(t *testing.T) {
	for _, backend := range liveBackends(t) {
		t.Run(backend, func(t *testing.T) {
			// Arrange
			m, _ := cachedLiveClient(t, backend)
			ctx := context.Background()

			// Act
			first, err1 := m.Chat(ctx, tinyChat("Reply with the single word: alpha"))
			second, err2 := m.Chat(ctx, tinyChat("Reply with the single word: alpha"))
			bumped := tinyChat("Reply with the single word: alpha")
			bumped.PromptVersion = "qa/v2"
			third, err3 := m.Chat(ctx, bumped)
			embed1, e1 := m.Embed(ctx, EmbedRequest{Input: []string{"cache me"}})
			embed2, e2 := m.Embed(ctx, EmbedRequest{Input: []string{"cache me"}})

			// Assert
			if err1 != nil || err2 != nil || err3 != nil || e1 != nil || e2 != nil {
				t.Fatalf("errors: %v %v %v %v %v", err1, err2, err3, e1, e2)
			}
			if first.Cached || !second.Cached || third.Cached {
				t.Errorf("cached flags = %v %v %v, want false true false (PromptVersion change must miss)", first.Cached, second.Cached, third.Cached)
			}
			if second.Text != first.Text {
				t.Errorf("cached text %q != original %q", second.Text, first.Text)
			}
			if embed1.Cached || !embed2.Cached || len(embed1.Vectors[0]) != len(embed2.Vectors[0]) || embed1.Vectors[0][0] != embed2.Vectors[0][0] {
				t.Errorf("embedding cache: cached=%v/%v dims=%d/%d", embed1.Cached, embed2.Cached, len(embed1.Vectors[0]), len(embed2.Vectors[0]))
			}
			if got := m.Spent().Calls; got != 3 {
				t.Errorf("provider calls = %d, want 3 (two cache hits cost nothing)", got)
			}
		})
	}
}

func TestLiveCacheTamperedPlantedAndSymlinkedEntriesMiss(t *testing.T) {
	for _, backend := range liveBackends(t) {
		t.Run(backend, func(t *testing.T) {
			// Arrange
			m, opts := cachedLiveClient(t, backend)
			ctx := context.Background()
			if _, err := m.Chat(ctx, tinyChat("Reply with the single word: bravo")); err != nil {
				t.Fatal(err)
			}
			files := cacheFiles(t, opts.CacheDir)
			if len(files) != 1 {
				t.Fatalf("want one cache entry, got %v", files)
			}
			var env entry
			raw, _ := os.ReadFile(files[0])
			if err := json.Unmarshal(raw, &env); err != nil {
				t.Fatal(err)
			}

			// Act: change the cached text but keep the MAC
			var payload map[string]any
			_ = json.Unmarshal(env.Payload, &payload)
			payload["text"] = "FORGED"
			env.Payload, _ = json.Marshal(payload)
			tampered, _ := json.Marshal(env)
			if err := os.WriteFile(files[0], tampered, 0o600); err != nil {
				t.Fatal(err)
			}
			resp, err := m.Chat(ctx, tinyChat("Reply with the single word: bravo"))

			// Assert: miss, real answer, entry replaced
			if err != nil || resp.Cached || resp.Text == "FORGED" {
				t.Fatalf("tampered entry served: %+v %v", resp, err)
			}
			if after, _ := os.ReadFile(files[0]); string(after) == string(tampered) {
				t.Error("tampered entry was not deleted/replaced")
			}
			if again, err := m.Chat(ctx, tinyChat("Reply with the single word: bravo")); err != nil || !again.Cached || again.Text == "FORGED" {
				t.Errorf("replacement entry not served from cache: %+v %v", again, err)
			}

			// Act: planted entry signed with another secret
			other := NewCache(opts.CacheDir, "attacker", []byte("0123456789abcdef0123456789abcdef"))
			planted := tinyChat("Reply with the single word: charlie")
			key, _ := m.Cache().key("chat", liveChatModelFor(backend), planted)
			other.store(key, ChatResponse{Text: "PLANTED", Model: "x"})
			// the real cache key for this request is the one the middleware computes; plant under that path
			pl, err := m.Chat(ctx, planted)
			if err != nil || pl.Text == "PLANTED" {
				t.Errorf("planted entry accepted: %+v %v", pl, err)
			}

			// Symlink at an entry path is dropped, never followed
			secretTarget := filepath.Join(t.TempDir(), "target.json")
			_ = os.WriteFile(secretTarget, tampered, 0o600)
			_ = os.Remove(files[0])
			testutil.SymlinkOrSkip(t, secretTarget, files[0])
			if resp, err := m.Chat(ctx, tinyChat("Reply with the single word: bravo")); err != nil || resp.Cached || resp.Text == "FORGED" {
				t.Errorf("symlinked entry served: %+v %v", resp, err)
			}
		})
	}
}

func liveChatModelFor(string) string { return liveConfig(BackendOpenAICompat).FullModel() }

func TestLiveCacheLooseSecretFileIsReplacedAndOldEntriesMiss(t *testing.T) {
	for _, backend := range liveBackends(t) {
		t.Run(backend, func(t *testing.T) {
			// Arrange
			m, opts := cachedLiveClient(t, backend)
			ctx := context.Background()
			if _, err := m.Chat(ctx, tinyChat("Reply with the single word: delta")); err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(opts.SecretPath)
			if err := os.Chmod(opts.SecretPath, 0o644); err != nil {
				t.Fatal(err)
			}

			// Act
			cfg := liveConfig(backend)
			cfg.Cache = ptr(true)
			m2, err := New(cfg, opts)
			if err != nil {
				t.Fatal(err)
			}
			defer m2.Close() //nolint:errcheck // test
			resp, cerr := m2.Chat(ctx, tinyChat("Reply with the single word: delta"))

			// Assert
			after, _ := os.ReadFile(opts.SecretPath)
			info, _ := os.Stat(opts.SecretPath)
			if cerr != nil || resp.Cached {
				t.Errorf("entry written under the loose secret was served: %+v %v", resp, cerr)
			}
			if string(before) == string(after) {
				t.Error("a group/world-readable secret was kept instead of replaced")
			}
			if info.Mode().Perm() != 0o600 {
				t.Errorf("secret mode = %o, want 600", info.Mode().Perm())
			}
		})
	}
}

func TestLiveBudgetStopsRunAndCountsOnlyRealCalls(t *testing.T) {
	for _, backend := range liveBackends(t) {
		t.Run(backend, func(t *testing.T) {
			// Arrange: 150 tokens in total; each call reserves ~prompt+16.
			cfg := liveConfig(backend)
			cfg.MaxTokens = 150
			m, err := New(cfg, Options{Getenv: os.Getenv})
			if err != nil {
				t.Fatal(err)
			}
			defer m.Close() //nolint:errcheck // test
			rec := &recorder{Client: m}

			// Act
			var stopped error
			calls := 0
			for range 30 {
				_, err := rec.Chat(context.Background(), ChatRequest{Messages: []Message{{Role: RoleUser, Content: "Reply with the single word: ok"}}, MaxTokens: 16, NoCache: true})
				if err != nil {
					stopped = err
					break
				}
				calls++
			}

			// Assert
			if !errors.Is(stopped, ErrBudget) {
				t.Fatalf("run was not stopped by the token budget: %v after %d calls", stopped, calls)
			}
			if sp := m.Spent(); sp.Tokens > 150 || sp.Calls != calls {
				t.Errorf("spent %+v after %d successful calls, want tokens <= 150 and calls == %d", sp, calls, calls)
			}
			t.Logf("%s: budget stopped after %d calls, spent=%+v", backend, calls, m.Spent())
		})
	}
}

func TestLiveBudgetMaxCallsAndUnknownPriceWithCostCap(t *testing.T) {
	for _, backend := range liveBackends(t) {
		t.Run(backend, func(t *testing.T) {
			// MaxCalls = 2: third call is refused before any request.
			cfg := liveConfig(backend)
			cfg.MaxCalls = 2
			m, err := New(cfg, Options{Getenv: os.Getenv})
			if err != nil {
				t.Fatal(err)
			}
			defer m.Close() //nolint:errcheck // test
			var errs []error
			for i := 0; i < 3; i++ {
				_, e := m.Chat(context.Background(), ChatRequest{Messages: []Message{{Role: RoleUser, Content: "Reply: ok"}}, MaxTokens: 8, NoCache: true})
				errs = append(errs, e)
			}
			if errs[0] != nil || errs[1] != nil || !errors.Is(errs[2], ErrBudget) {
				t.Errorf("max_calls=2: errors = %v", errs)
			}

			// A cost cap with a model the price table does not know must refuse, not guess.
			cfg = liveConfig(backend)
			cfg.MaxCostUSD = 0.5
			m2, err := New(cfg, Options{Getenv: os.Getenv})
			if err != nil {
				t.Fatal(err)
			}
			defer m2.Close() //nolint:errcheck // test
			_, e := m2.Chat(context.Background(), ChatRequest{Messages: []Message{{Role: RoleUser, Content: "Reply: ok"}}, MaxTokens: 8, NoCache: true})
			if !errors.Is(e, ErrBudget) {
				t.Errorf("cost cap with unknown model price: err = %v, want ErrBudget", e)
			}

			// With a price override the cap works: tiny cap refuses the worst case, a larger one admits it.
			cfg.PriceInputPerMTok, cfg.PriceOutputPerMTok = 0.1, 0.4
			cfg.MaxCostUSD = 0.0000001
			m3, _ := New(cfg, Options{Getenv: os.Getenv})
			defer m3.Close() //nolint:errcheck // test
			if _, e := m3.Chat(context.Background(), ChatRequest{Messages: []Message{{Role: RoleUser, Content: "Reply: ok"}}, MaxTokens: 8, NoCache: true}); !errors.Is(e, ErrBudget) {
				t.Errorf("tiny cap: err = %v, want ErrBudget", e)
			}
			cfg.MaxCostUSD = 0.01
			m4, _ := New(cfg, Options{Getenv: os.Getenv})
			defer m4.Close() //nolint:errcheck // test
			resp, e := m4.Chat(context.Background(), ChatRequest{Messages: []Message{{Role: RoleUser, Content: "Reply: ok"}}, MaxTokens: 8, NoCache: true})
			if e != nil || !resp.CostKnown || resp.CostUSD <= 0 || resp.CostUSD > 0.01 {
				t.Errorf("override price: %+v %v", resp, e)
			}
			if sp := m4.Spent(); sp.CostUSD <= 0 || sp.CostUSD > 0.01 {
				t.Errorf("spent cost = %v", sp.CostUSD)
			}
		})
	}
}

func TestLiveTypedErrors(t *testing.T) {
	for _, backend := range liveBackends(t) {
		t.Run(backend, func(t *testing.T) {
			ctx := context.Background()
			req := ChatRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}, MaxTokens: 8, NoCache: true}

			t.Run("bad key is permanent and not retried", func(t *testing.T) {
				cfg := liveConfig(backend)
				cfg.MaxRetries = 3
				cfg.MaxCalls = 100
				m, _ := New(cfg, Options{Getenv: func(string) string { return "AIzaSyD-not-a-real-key-0000000000000000" }})
				defer m.Close() //nolint:errcheck // test
				start := time.Now()
				_, err := m.Chat(ctx, req)
				var le *Error
				if !errors.As(err, &le) || IsTransient(err) {
					t.Fatalf("err = %v, want permanent *Error", err)
				}
				if le.Kind != KindAuth && le.Kind != KindProvider {
					t.Errorf("kind = %q", le.Kind)
				}
				if got := m.Spent().Calls; got != 1 {
					t.Errorf("calls = %d, want 1 (no retry of a permanent error)", got)
				}
				if strings.Contains(err.Error(), "not-a-real-key") {
					t.Errorf("error leaks the key: %v", err)
				}
				t.Logf("bad key: kind=%s status=%d %.1fs", le.Kind, le.Status, time.Since(start).Seconds())
			})

			t.Run("1ms timeout is a timeout", func(t *testing.T) {
				m := liveClient(t, backend, os.Getenv)
				r := req
				r.Timeout = time.Millisecond
				_, err := m.Chat(ctx, r)
				if !errors.Is(err, ErrTimeout) && !errors.Is(err, context.DeadlineExceeded) {
					t.Errorf("err = %v, want a timeout", err)
				}
			})

			t.Run("cancellation returns promptly", func(t *testing.T) {
				m := liveClient(t, backend, os.Getenv)
				cctx, cancel := context.WithCancel(ctx)
				go func() { time.Sleep(30 * time.Millisecond); cancel() }()
				start := time.Now()
				_, err := m.Chat(cctx, ChatRequest{Messages: []Message{{Role: RoleUser, Content: "Write 400 words about rivers."}}, MaxTokens: 600, NoCache: true})
				if err == nil {
					t.Skip("provider answered before the cancel")
				}
				if time.Since(start) > 5*time.Second {
					t.Errorf("cancel took %v", time.Since(start))
				}
				if !errors.Is(err, context.Canceled) && !errors.Is(err, ErrTimeout) {
					t.Errorf("err = %v, want context.Canceled", err)
				}
			})

			t.Run("unknown model is a permanent provider error", func(t *testing.T) {
				cfg := liveConfig(backend)
				cfg.Model = "gemini-no-such-model-9"
				m, _ := New(cfg, Options{Getenv: os.Getenv})
				defer m.Close() //nolint:errcheck // test
				_, err := m.Chat(ctx, req)
				if err == nil || IsTransient(err) {
					t.Errorf("err = %v, want a permanent error", err)
				}
				var le *Error
				if errors.As(err, &le) {
					t.Logf("unknown model: kind=%s status=%d", le.Kind, le.Status)
				}
			})

			t.Run("embedding of empty input is refused or empty, never a panic", func(t *testing.T) {
				m := liveClient(t, backend, os.Getenv)
				resp, err := m.Embed(ctx, EmbedRequest{Input: nil, NoCache: true})
				t.Logf("empty embed: vectors=%d err=%v", len(resp.Vectors), err)
				if err == nil && len(resp.Vectors) != 0 {
					t.Errorf("vectors for no input: %d", len(resp.Vectors))
				}
			})

			t.Run("embedding batch keeps input order and dimension", func(t *testing.T) {
				m := liveClient(t, backend, os.Getenv)
				resp, err := m.Embed(ctx, EmbedRequest{Input: []string{"cats", "dogs", "the stock market"}, NoCache: true})
				if err != nil || len(resp.Vectors) != 3 {
					t.Fatalf("%v %d", err, len(resp.Vectors))
				}
				if len(resp.Vectors[0]) != len(resp.Vectors[2]) || len(resp.Vectors[0]) == 0 {
					t.Errorf("dims %d %d", len(resp.Vectors[0]), len(resp.Vectors[2]))
				}
				if cos(resp.Vectors[0], resp.Vectors[1]) <= cos(resp.Vectors[0], resp.Vectors[2]) {
					t.Errorf("cats~dogs (%.3f) should beat cats~stock market (%.3f): order or content mixed up", cos(resp.Vectors[0], resp.Vectors[1]), cos(resp.Vectors[0], resp.Vectors[2]))
				}
			})
		})
	}
}

func cos(a, b []float32) float64 {
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	return dot / (sqrt(na) * sqrt(nb))
}

func sqrt(x float64) float64 {
	z := x / 2
	for i := 0; i < 40 && z > 0; i++ {
		z = (z + x/z) / 2
	}
	return z
}

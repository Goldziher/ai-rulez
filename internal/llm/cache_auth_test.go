package llm

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

func cacheFixture(t *testing.T) (m *Managed, f *Fake, opts Options) {
	t.Helper()
	root := t.TempDir()
	opts = Options{ConfigDir: filepath.Join(root, "repo", ".ai-rulez"), CacheDir: filepath.Join(root, "cache"), SecretPath: filepath.Join(root, "cfg", "llm-cache.key")}
	f = NewFake()
	return Wrap(f, allowed(Config{Model: "gpt-4o-mini"}), opts), f, opts
}

func entryFiles(t *testing.T, dir string) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestCacheRejectsTamperedTruncatedAndPlantedEntries(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name   string
		mutate func(t *testing.T, path string)
	}{
		{"payload edited", func(t *testing.T, p string) {
			b, _ := os.ReadFile(p) //nolint:errcheck,gosec // test
			if err := os.WriteFile(p, []byte(strings.Replace(string(b), "fake", "FORGED", 1)), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{"truncated", func(t *testing.T, p string) {
			b, _ := os.ReadFile(p) //nolint:errcheck,gosec // test
			if err := os.WriteFile(p, b[:len(b)/2], 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{"unauthenticated legacy entry", func(t *testing.T, p string) {
			if err := os.WriteFile(p, []byte(`{"text":"planted","model":"m"}`), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{"planted with a wrong secret", func(t *testing.T, p string) {
			forged := NewCache(filepath.Dir(filepath.Dir(p)), "x", []byte("0123456789abcdef0123456789abcdef"))
			key := strings.TrimSuffix(filepath.Base(p), ".json")
			forged.store(key, ChatResponse{Text: "planted"})
		}},
		{"oversized", func(t *testing.T, p string) {
			if err := os.WriteFile(p, make([]byte, maxCacheEntryBytes+10), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange: one genuine cached reply
			m, f, opts := cacheFixture(t)
			first, err := m.Chat(ctx, chatReq("q"))
			if err != nil {
				t.Fatal(err)
			}
			files := entryFiles(t, CacheDirFor(opts))
			if len(files) != 1 {
				t.Fatalf("want 1 entry, got %d", len(files))
			}
			// Act
			tt.mutate(t, files[0])
			got, err := m.Chat(ctx, chatReq("q"))
			// Assert: never served, replaced by a fresh genuine entry
			if err != nil || got.Cached || got.Text != first.Text || len(f.ChatCalls()) != 2 {
				t.Fatalf("bad entry must miss: %+v err=%v calls=%d", got, err, len(f.ChatCalls()))
			}
			again, _ := m.Chat(ctx, chatReq("q"))
			if !again.Cached || again.Text != first.Text {
				t.Fatalf("the replaced entry must hit: %+v", again)
			}
		})
	}
}

func TestCacheSecretCreatedOnFirstUseWith0600(t *testing.T) {
	m, _, opts := cacheFixture(t)
	if _, err := m.Chat(context.Background(), chatReq("q")); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(opts.SecretPath)
	if err != nil || fi.Size() != cacheSecretBytes {
		t.Fatalf("secret not created: %v %v", fi, err)
	}
	if runtime.GOOS != "windows" {
		if fi.Mode().Perm() != 0o600 {
			t.Errorf("secret mode %v, want 0600", fi.Mode().Perm())
		}
		if di, _ := os.Stat(filepath.Dir(opts.SecretPath)); di.Mode().Perm() != 0o700 { //nolint:errcheck // test
			t.Errorf("secret dir mode %v, want 0700", di.Mode().Perm())
		}
		if di, _ := os.Stat(CacheDirFor(opts)); di.Mode().Perm() != 0o700 { //nolint:errcheck // test
			t.Errorf("cache dir mode %v, want 0700", di.Mode().Perm())
		}
	}
}

func TestCacheMissingOrCorruptSecretInvalidatesEntries(t *testing.T) {
	ctx := context.Background()
	for name, mutate := range map[string]func(string){
		"deleted":   func(p string) { os.Remove(p) },                            //nolint:errcheck,gosec // test
		"truncated": func(p string) { os.WriteFile(p, []byte("short"), 0o600) }, //nolint:errcheck,gosec // test
	} {
		t.Run(name, func(t *testing.T) {
			m, _, opts := cacheFixture(t)
			if _, err := m.Chat(ctx, chatReq("q")); err != nil {
				t.Fatal(err)
			}
			mutate(opts.SecretPath)
			f2 := NewFake()
			m2 := Wrap(f2, allowed(Config{Model: "gpt-4o-mini"}), opts)
			r, err := m2.Chat(ctx, chatReq("q"))
			if err != nil || r.Cached || len(f2.ChatCalls()) != 1 {
				t.Fatalf("a new secret must invalidate old entries: %+v %v", r, err)
			}
			if b, err := os.ReadFile(opts.SecretPath); err != nil || len(b) != cacheSecretBytes { //nolint:gosec // test
				t.Fatalf("secret not regenerated: %d %v", len(b), err)
			}
		})
	}
}

func TestCacheDisabledWhenSecretCannotBeCreated(t *testing.T) {
	root := t.TempDir()
	blocker := filepath.Join(root, "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	opts := Options{ConfigDir: root, CacheDir: filepath.Join(root, "cache"), SecretPath: filepath.Join(blocker, "key")}
	m := Wrap(NewFake(), allowed(Config{Model: "m"}), opts)
	if m.Cache() != nil {
		t.Fatal("no usable secret: the cache must be off, not unauthenticated")
	}
	if _, err := os.Stat(opts.CacheDir); err == nil {
		t.Fatal("nothing may be written without a secret")
	}
}

func TestCacheIgnoresSymlinkedEntry(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on windows")
	}
	ctx := context.Background()
	m, _, opts := cacheFixture(t)
	if _, err := m.Chat(ctx, chatReq("q")); err != nil {
		t.Fatal(err)
	}
	files := entryFiles(t, CacheDirFor(opts))
	target := filepath.Join(t.TempDir(), "elsewhere.json")
	b, _ := os.ReadFile(files[0]) //nolint:errcheck,gosec // test
	if err := os.WriteFile(target, b, 0o600); err != nil {
		t.Fatal(err)
	}
	os.Remove(files[0]) //nolint:errcheck,gosec // test
	testutil.SymlinkOrSkip(t, target, files[0])
	if r, _ := m.Chat(ctx, chatReq("q")); r.Cached { //nolint:errcheck // test
		t.Fatal("a symlinked entry must not be followed")
	}
}

func TestCacheIdentityChangesWithHostKeyAndPrices(t *testing.T) {
	base := Config{Provider: "openai", BaseURL: "https://gw.example/v1", APIKeyEnv: "KEY_A"}
	variants := map[string]func(*Config){
		"base":     func(*Config) {},
		"host":     func(c *Config) { c.BaseURL = "https://gw2.example/v1" },
		"scheme":   func(c *Config) { c.BaseURL = "http://gw.example/v1" },
		"keyenv":   func(c *Config) { c.APIKeyEnv = "KEY_B" },
		"provider": func(c *Config) { c.Provider = "gemini" },
		"prices":   func(c *Config) { c.PriceInputPerMTok = 1 },
	}
	seen := map[string]string{}
	for name, mod := range variants {
		c := base
		mod(&c)
		id := cacheIdentity(c)
		if prev, dup := seen[id]; dup {
			t.Errorf("%s and %s share identity %q", name, prev, id)
		}
		seen[id] = name
		if strings.Contains(id, "sk-") {
			t.Errorf("identity must hold names, never values: %q", id)
		}
	}
}

func TestCacheDirIsNamespacedPerProjectOutsideRepo(t *testing.T) {
	a := CacheDirFor(Options{ConfigDir: "/work/a/.ai-rulez"})
	b := CacheDirFor(Options{ConfigDir: "/work/b/.ai-rulez"})
	if a == "" || a == b || strings.HasPrefix(a, "/work/") {
		t.Fatalf("cache dirs: %q %q", a, b)
	}
	if CacheDirFor(Options{}) != "" {
		t.Fatal("no config dir, no cache")
	}
}

func TestLoadOrCreateSecret_ReplacesUnusableFilesAndRefusesSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission and symlink semantics differ on windows")
	}
	dir := filepath.Join(t.TempDir(), "cfg")
	path := filepath.Join(dir, "llm-cache.key")
	first, err := loadOrCreateSecret(path)
	if err != nil || len(first) != cacheSecretBytes {
		t.Fatalf("create: %v", err)
	}
	again, err := loadOrCreateSecret(path)
	if err != nil || string(again) != string(first) {
		t.Fatalf("a valid secret must be reused: %v", err)
	}
	// loose mode: not trusted, replaced
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	replaced, err := loadOrCreateSecret(path)
	if err != nil || string(replaced) == string(first) {
		t.Fatalf("a world-readable secret must be replaced: %v", err)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 { //nolint:errcheck // test
		t.Fatalf("replacement mode = %v", info.Mode().Perm())
	}
	// symlink: refused, target untouched
	victim := filepath.Join(t.TempDir(), "victim")
	if err := os.WriteFile(victim, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	testutil.SymlinkOrSkip(t, victim, path)
	if _, err := loadOrCreateSecret(path); err == nil {
		t.Fatal("a symlinked secret must be refused")
	}
	if b, _ := os.ReadFile(victim); string(b) != "keep" { //nolint:errcheck,gosec // test
		t.Fatal("the symlink target was modified")
	}
}

func TestLoadOrCreateSecret_RefusesAGroupWritableDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission semantics differ on windows")
	}
	dir := filepath.Join(t.TempDir(), "cfg")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o775); err != nil {
		t.Fatal(err)
	}
	if _, err := loadOrCreateSecret(filepath.Join(dir, "k")); err == nil {
		t.Fatal("a group-writable secret directory must be refused")
	}
}

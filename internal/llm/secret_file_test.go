package llm

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

func TestLoadSecretFileCreatesAndReusesPrivateSecret(t *testing.T) {
	// Arrange
	path := filepath.Join(t.TempDir(), "ai-rulez", "key")

	// Act
	first, err := LoadSecretFile(path)
	if err != nil {
		t.Fatalf("first load: %v", err)
	}
	second, err := LoadSecretFile(path)
	if err != nil {
		t.Fatalf("second load: %v", err)
	}

	// Assert
	if len(first) != cacheSecretBytes {
		t.Fatalf("secret length = %d, want %d", len(first), cacheSecretBytes)
	}
	if !bytes.Equal(first, second) {
		t.Error("a second load must return the secret created by the first")
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Errorf("secret mode = %o, want 600", perm)
		}
	}
}

func TestLoadSecretFileReplacesAnUnusableSecret(t *testing.T) {
	tests := []struct {
		name  string
		write func(t *testing.T, path string)
	}{
		{"empty", func(t *testing.T, path string) { writeFile(t, path, nil, 0o600) }},
		{"too short", func(t *testing.T, path string) { writeFile(t, path, []byte("short"), 0o600) }},
		{"too long", func(t *testing.T, path string) {
			writeFile(t, path, bytes.Repeat([]byte{1}, cacheSecretBytes+1), 0o600)
		}},
		{"group readable", func(t *testing.T, path string) {
			if runtime.GOOS == "windows" {
				t.Skip("file modes carry no meaning on Windows")
			}
			writeFile(t, path, bytes.Repeat([]byte{7}, cacheSecretBytes), 0o640)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			path := filepath.Join(t.TempDir(), "key")
			tt.write(t, path)
			old, _ := os.ReadFile(path) //nolint:errcheck // test fixture

			// Act
			got, err := LoadSecretFile(path)

			// Assert
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			if len(got) != cacheSecretBytes {
				t.Fatalf("secret length = %d, want %d", len(got), cacheSecretBytes)
			}
			if bytes.Equal(got, old) {
				t.Error("an unusable secret must be replaced, not trusted")
			}
			again, err := LoadSecretFile(path)
			if err != nil || !bytes.Equal(again, got) {
				t.Errorf("the replacement must persist: err=%v", err)
			}
		})
	}
}

func TestLoadSecretFileRefusesASymlink(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	writeFile(t, target, bytes.Repeat([]byte{9}, cacheSecretBytes), 0o600)
	link := filepath.Join(dir, "key")
	testutil.SymlinkOrSkip(t, target, link)

	// Act
	_, err := LoadSecretFile(link)

	// Assert
	if !errors.Is(err, errSecretNotRegular) {
		t.Fatalf("err = %v, want errSecretNotRegular", err)
	}
}

func TestLoadSecretFileRefusesAnOpenDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory modes carry no meaning on Windows")
	}
	// Arrange
	dir := filepath.Join(t.TempDir(), "open")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o777); err != nil { //nolint:gosec // the test needs a world-writable directory
		t.Fatal(err)
	}

	// Act
	_, err := LoadSecretFile(filepath.Join(dir, "key"))

	// Assert
	if err == nil || !strings.Contains(err.Error(), "writable by group or others") {
		t.Fatalf("err = %v, want a refusal of the open directory", err)
	}
}

func TestLoadSecretFileEmptyPath(t *testing.T) {
	if _, err := LoadSecretFile(""); err == nil {
		t.Fatal("an empty path (no home directory) must be an error")
	}
}

func writeFile(t *testing.T, path string, data []byte, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, data, mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func TestScrubDropsKeysAndURLQueries(t *testing.T) {
	const token = "ghp_abcdefghijklmnopqrstuvwxyz0123456789"
	l := &literLLM{key: "plain-key-value-1234"}
	tests := []struct {
		name        string
		in          string
		wantContain string
		forbid      []string
	}{
		{"query token is dropped", "POST https://api.example.com/v1/chat?key=abc123secret failed: connection refused", "connection refused", []string{"abc123secret", "key="}},
		{"configured key is removed by value", "bad header for plain-key-value-1234", "bad header", []string{"plain-key-value-1234"}},
		{"key-shaped text is redacted", "dial failed using " + token, "dial failed", []string{token}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := l.scrub(tt.in)
			if !strings.Contains(got, tt.wantContain) {
				t.Errorf("scrub = %q, want it to contain %q", got, tt.wantContain)
			}
			for _, f := range tt.forbid {
				if strings.Contains(got, f) {
					t.Errorf("scrub = %q, must not contain %q", got, f)
				}
			}
		})
	}
}

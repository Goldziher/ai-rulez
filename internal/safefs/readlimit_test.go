package safefs

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"
)

func TestReadLimited(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		limit   int64
		want    string
		tooLong bool
	}{
		{"under the limit", "abc", 10, "abc", false},
		{"exactly the limit", "abcde", 5, "abcde", false},
		{"one byte over", "abcdef", 5, "", true},
		{"empty", "", 5, "", false},
		{"zero limit with data", "a", 0, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReadLimited(strings.NewReader(tt.in), tt.limit)
			if tt.tooLong {
				if !errors.Is(err, ErrTooLarge) {
					t.Fatalf("err = %v, want ErrTooLarge", err)
				}
				if got != nil {
					t.Errorf("a refused read must not return a truncated prefix, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if !bytes.Equal(got, []byte(tt.want)) {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestReadLimitedPropagatesReaderErrors(t *testing.T) {
	boom := errors.New("boom")
	_, err := ReadLimited(iotest.ErrReader(boom), 10)
	if !errors.Is(err, boom) || errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v, want the reader's error", err)
	}
}

func TestReadFileLimited(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}

	t.Run("a file within the limit is read whole", func(t *testing.T) {
		got, err := ReadFileLimited(write("ok", "hello"), 5)
		if err != nil || string(got) != "hello" {
			t.Fatalf("got %q, %v", got, err)
		}
	})
	t.Run("a longer file is refused, not cut", func(t *testing.T) {
		p := write("long", "hello!")
		got, err := ReadFileLimited(p, 5)
		if !errors.Is(err, ErrTooLarge) || got != nil {
			t.Fatalf("got %q, %v; want ErrTooLarge", got, err)
		}
		if !strings.Contains(err.Error(), p) {
			t.Errorf("error %q does not name the path", err)
		}
	})
	t.Run("a missing file keeps os.ErrNotExist", func(t *testing.T) {
		if _, err := ReadFileLimited(filepath.Join(dir, "missing"), 5); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("err = %v, want os.ErrNotExist", err)
		}
	})
}

package cache

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/tools/skillbench/internal/scanner"
)

func key() Key {
	return Key{Tool: "t", Version: "1.0.0", Adapter: "abc", Environment: "arm64-none-x"}
}

func TestRoundTripIsStable(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, key())
	if err != nil {
		t.Fatal(err)
	}
	s.Put("b", scanner.Verdict{Flagged: true, MaxSeverity: "high", Rules: []string{"R1"}, Findings: 1})
	s.Put("a", scanner.Verdict{Rules: []string{}})
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(s.path)
	if err != nil {
		t.Fatal(err)
	}
	again, err := Open(dir, key())
	if err != nil {
		t.Fatal(err)
	}
	if again.Len() != 2 {
		t.Fatalf("len = %d", again.Len())
	}
	if v, ok := again.Get("b"); !ok || !v.Flagged || v.Rules[0] != "R1" {
		t.Fatalf("got %+v %v", v, ok)
	}
	again.Put("a", scanner.Verdict{Rules: []string{}})
	if err := again.Save(); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(s.path)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("same verdicts must give the same bytes")
	}
}

func TestEveryKeyPartSeparatesStores(t *testing.T) {
	base := key()
	for _, k := range []Key{
		{"x", base.Version, base.Adapter, base.Environment},
		{base.Tool, "2.0.0", base.Adapter, base.Environment},
		{base.Tool, base.Version, "def", base.Environment},
		{base.Tool, base.Version, base.Adapter, "amd64-none-x"},
	} {
		if k.file() == base.file() {
			t.Fatalf("%+v shares a file with the base key", k)
		}
	}
}

func TestIncompleteKeyIsRefused(t *testing.T) {
	if _, err := Open(t.TempDir(), Key{Tool: "t"}); err == nil {
		t.Fatal("an incomplete key must not open a store")
	}
}

func TestHashDirCoversPathsAndBytes(t *testing.T) {
	write := func(dir, name, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	a, b, c := t.TempDir(), t.TempDir(), t.TempDir()
	write(a, "SKILL.md", "one")
	write(b, "SKILL.md", "one")
	write(c, "OTHER.md", "one")
	ha, _ := HashDir(a)
	hb, _ := HashDir(b)
	hc, _ := HashDir(c)
	if ha != hb {
		t.Fatal("same tree, same digest")
	}
	if ha == hc {
		t.Fatal("a renamed file must change the digest")
	}
	write(b, "SKILL.md", "two")
	if hb2, _ := HashDir(b); hb2 == ha {
		t.Fatal("changed bytes must change the digest")
	}
}

func TestErrorIsKeptButNeverReplacesAnAnswer(t *testing.T) {
	s, err := Open(t.TempDir(), key())
	if err != nil {
		t.Fatal(err)
	}
	s.Put("a", scanner.Verdict{Error: "timeout"})
	if v, ok := s.Get("a"); !ok || v.Error != "timeout" {
		t.Fatalf("got %+v %v", v, ok)
	}
	s.Put("a", scanner.Verdict{Flagged: true})
	s.Put("a", scanner.Verdict{Error: "timeout"})
	if v, _ := s.Get("a"); !v.Flagged || v.Error != "" {
		t.Fatalf("an error replaced an answer: %+v", v)
	}
}

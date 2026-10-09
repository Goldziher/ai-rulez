package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

const nativeFM = "---\npriority: high\ntargets: [claude, cursor]\ndescription: Shared\nlicense: MIT\nmetadata:\n  owner: team\nwhen_to_use: always\nenabled: true\n---\nBody\n"

const okfFM = `---
type: Decision
title: Native Rule
description: Shared
x-ai-rulez:
  kind: rule
  id: native
  metadata:
    priority: high
    targets:
    - claude
    - cursor
    license: MIT
    metadata:
      owner: team
    when_to_use: always
    enabled: true
---
Body
`

func TestOKFFrontmatterEqualsNative(t *testing.T) {
	native, nativeBody, _ := parseFrontmatter(nativeFM)
	okf, okfBody, malformed := parseFrontmatter(okfFM)
	if malformed || native == nil || okf == nil {
		t.Fatalf("parse failed: %v %v %v", malformed, native, okf)
	}
	if nativeBody != okfBody {
		t.Fatalf("body differs: %q vs %q", nativeBody, okfBody)
	}
	if okf.OKFType == "" || okf.OKFTitle == "" {
		t.Errorf("type and title were dropped: %q %q", okf.OKFType, okf.OKFTitle)
	}
	okf.OKFType, okf.OKFTitle = "", ""
	if !reflect.DeepEqual(native, okf) {
		t.Fatalf("metadata differs\nnative: %#v\nokf:    %#v", native, okf)
	}
	for _, key := range []string{"type", "title", "x-ai-rulez"} {
		if _, ok := okf.Extra[key]; ok {
			t.Errorf("reserved key %q leaked into Extra", key)
		}
	}
}

func TestOKFFrontmatterWithoutExtensionKeepsNothingReserved(t *testing.T) {
	m, _, _ := parseFrontmatter("---\ntype: Concept\ntitle: T\npriority: low\n---\nx\n")
	if m == nil || m.Priority != "low" || len(m.Extra) != 0 {
		t.Fatalf("unexpected metadata: %#v", m)
	}
}

func TestNativeFrontmatterUntouched(t *testing.T) {
	in := "priority: high\nfoo: bar"
	if got := normalizeOKFFrontmatter(in); got != in {
		t.Fatalf("native frontmatter was rewritten: %q", got)
	}
	if got := normalizeOKFFrontmatter("note: the type of title\n"); got != "note: the type of title\n" {
		t.Fatalf("a value containing a reserved word was rewritten: %q", got)
	}
}

func TestOKFIndexFilesAreNotContent(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "rules/index.md", "# Concepts\n\n* [Real](real.md) - Real\n")
	writeTestFile(t, dir, "rules/log.md", "# Log\n\n## 2026-10-01\n\n* [Real](real.md) - changed\n")
	writeTestFile(t, dir, "rules/real.md", "# Real\n")
	tree, err := ScanContentTree(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(tree.Rules) != 1 || tree.Rules[0].Name != "real" {
		t.Fatalf("rules = %+v", tree.Rules)
	}
}

func writeTestFile(t *testing.T, dir, rel, body string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestProseIndexFileStaysContent(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "rules/index.md", "# Index\n\nHow to find things.\n")
	tree, err := ScanContentTree(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(tree.Rules) != 1 || tree.Rules[0].Name != "index" {
		t.Fatalf("a rule named index was dropped: %+v", tree.Rules)
	}
}

// NativeContent uses the shared frontmatter splitter, so it agrees with the
// loader about a BOM, an opener or closer with trailing spaces, and CRLF.
func TestNativeContentSharesTheFrontmatterSplitter(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"BOM before the fence", "\xef\xbb\xbf---\ntype: Rule\ntitle: T\npriority: high\n---\nbody\n", "---\npriority: high\n---\nbody\n"},
		{"opener with trailing space", "--- \ntype: Rule\ntitle: T\npriority: high\n---\nbody\n", "---\npriority: high\n---\nbody\n"},
		{"closer with trailing space", "---\ntype: Rule\ntitle: T\npriority: high\n--- \nbody\n", "---\npriority: high\n---\nbody\n"},
		{"CRLF", "---\r\ntype: Rule\r\ntitle: T\r\npriority: high\r\n---\r\nbody\r\n", "---\npriority: high\n---\nbody\r\n"},
		{"only reserved keys leaves an empty mapping", "---\ntype: Rule\ntitle: T\n---\nbody\n", "---\n{}\n---\nbody\n"},
		{"no frontmatter", "# Title\n", "# Title\n"},
		{"unclosed block is returned as it is", "---\ntype: Rule\nbody\n", "---\ntype: Rule\nbody\n"},
		{"a table rule does not close the block", "---\ntype: Rule\npriority: high\n---\nbody\n\n---\nnot closing\n", "---\npriority: high\n---\nbody\n\n---\nnot closing\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NativeContent(tt.in); got != tt.want {
				t.Fatalf("NativeContent(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

package dataset

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func archive(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range entries {
		h := &tar.Header{Name: "repo-abc/" + name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	link := &tar.Header{Name: "repo-abc/skills/one/link", Linkname: "/etc/passwd", Typeflag: tar.TypeSymlink}
	if err := tw.WriteHeader(link); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestMaterializeClean(t *testing.T) {
	body := archive(t, map[string]string{
		"skills/one/SKILL.md":       "one",
		"skills/one/scripts/run.sh": "#!/bin/sh\n",
		"skills/two/SKILL.md":       "two",
		"README.md":                 "not a skill",
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(body) }))
	defer srv.Close()
	data := t.TempDir()
	p := &Protocol{Clean: Clean{Repos: []CleanRepo{{Repo: "acme/skills", Commit: "abc"}}}}
	if err := fetchCleanFrom(t.Context(), p, data, srv.Client(), srv.URL+"/"); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	m, err := MaterializeClean(p, data, TrackClean, out, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Samples) != 2 || m.Samples[0].ID != "acme__skills__skills__one" || m.Samples[0].Malicious {
		t.Fatalf("samples = %+v", m.Samples)
	}
	script := filepath.Join(out, "acme__skills__skills__one", "scripts", "run.sh")
	info, err := os.Stat(script)
	if err != nil || info.Mode().Perm()&0o111 != 0 {
		t.Fatalf("script = %v, %v", info, err)
	}
	if _, err := os.Lstat(filepath.Join(out, "acme__skills__skills__one", "link")); !os.IsNotExist(err) {
		t.Fatal("links must be skipped")
	}
	p.Clean.Population = "0"
	if _, err := MaterializeClean(p, data, TrackClean, t.TempDir(), 1); err == nil {
		t.Fatal("a changed population must fail")
	}
}

func TestSafeRelative(t *testing.T) {
	for name, want := range map[string]bool{"a/b": true, "../x": false, "/etc/passwd": false, "": false} {
		if safeRelative(name) != want {
			t.Errorf("safeRelative(%q) != %v", name, want)
		}
	}
}

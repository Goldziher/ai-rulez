package workspace_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
	"github.com/Goldziher/ai-rulez/v5/internal/workspace"
)

// build lays out the same tree in a Mem and an OS workspace.
//
//	a.txt            file
//	dir/b.txt        file
//	dir/up           symlink -> ../a.txt   (inside)
//	dir/abs          symlink -> <root>/a.txt (absolute, inside)
//	dir/escape       symlink -> ../../outside.txt
//	loop1, loop2     symlinks to each other
func build(t *testing.T) map[string]workspace.Workspace {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{"a.txt": "a", "dir/b.txt": "b"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	real := dir
	links := map[string]string{
		"dir/up":     "../a.txt",
		"dir/abs":    filepath.Join(real, "a.txt"),
		"dir/escape": "../../outside.txt",
		"loop1":      "loop2",
		"loop2":      "loop1",
	}
	mem := workspace.NewMem(real)
	mem.Set("a.txt", "a", 0o644)
	mem.Set("dir/b.txt", "b", 0o644)
	for name, target := range links {
		testutil.SymlinkOrSkip(t, target, filepath.Join(real, filepath.FromSlash(name)))
		mem.Symlink(name, target)
	}
	osws, err := workspace.OS(real)
	if err != nil {
		t.Fatal(err)
	}
	return map[string]workspace.Workspace{"os": osws, "mem": mem}
}

func TestResolve(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr error
	}{
		{name: "plain file", in: "dir/b.txt", want: "dir/b.txt"},
		{name: "relative link inside", in: "dir/up", want: "a.txt"},
		{name: "absolute link inside", in: "dir/abs", want: "a.txt"},
		{name: "link out of the root", in: "dir/escape", wantErr: workspace.ErrOutside},
		{name: "root", in: ".", want: "."},
		{name: "link cycle", in: "loop1", wantErr: workspace.ErrTooManyLinks},
		{name: "missing", in: "dir/none", wantErr: fs.ErrNotExist},
	}
	for kind, ws := range build(t) {
		for _, tc := range tests {
			t.Run(kind+"/"+tc.name, func(t *testing.T) {
				got, err := workspace.Resolve(ws, tc.in)
				if tc.wantErr != nil {
					if err == nil {
						t.Fatalf("Resolve(%q) = %q, want error %v", tc.in, got, tc.wantErr)
					}
					if !errors.Is(err, tc.wantErr) {
						t.Fatalf("Resolve(%q) error = %v, want %v", tc.in, err, tc.wantErr)
					}
					return
				}
				if err != nil || got != tc.want {
					t.Fatalf("Resolve(%q) = %q, %v; want %q", tc.in, got, err, tc.want)
				}
			})
		}
	}
}

func TestViewMapsAbsolutePathsAndNamesThemInErrors(t *testing.T) {
	for kind, ws := range build(t) {
		t.Run(kind, func(t *testing.T) {
			v := workspace.NewView(ws)
			data, err := v.ReadFile(filepath.Join(ws.Root(), "dir", "b.txt"))
			if err != nil || string(data) != "b" {
				t.Fatalf("ReadFile = %q, %v", data, err)
			}
			info, err := v.Lstat(filepath.Join(ws.Root(), "dir", "up"))
			if err != nil || info.Mode()&fs.ModeSymlink == 0 {
				t.Fatalf("Lstat of a symlink = %v, %v", info, err)
			}
			entries, err := v.ReadDir(filepath.Join(ws.Root(), "dir"))
			if err != nil || len(entries) != 4 {
				t.Fatalf("ReadDir = %d entries, %v", len(entries), err)
			}

			missing := filepath.Join(ws.Root(), "nope.txt")
			_, err = v.ReadFile(missing)
			var pe *fs.PathError
			if !errors.Is(err, fs.ErrNotExist) || !errors.As(err, &pe) || pe.Path != missing {
				t.Fatalf("a missing file error = %v, want ErrNotExist naming %s", err, missing)
			}

			if _, err = v.Stat(filepath.Join(filepath.Dir(ws.Root()), "other")); !errors.Is(err, workspace.ErrOutside) {
				t.Fatalf("a path outside the root = %v, want ErrOutside", err)
			}
			if got, err := v.EvalSymlinks(filepath.Join(ws.Root(), "dir", "up")); err != nil || got != filepath.Join(ws.Root(), "a.txt") {
				t.Fatalf("EvalSymlinks = %q, %v", got, err)
			}
			if _, err = v.EvalSymlinks(filepath.Join(ws.Root(), "dir", "escape")); !errors.Is(err, workspace.ErrOutside) {
				t.Fatalf("EvalSymlinks of an escaping link = %v, want ErrOutside", err)
			}
		})
	}
}

func TestZeroViewFailsClosed(t *testing.T) {
	var v workspace.View
	if _, err := v.ReadFile("/anything"); !errors.Is(err, workspace.ErrNoWorkspace) {
		t.Fatalf("zero View ReadFile = %v, want ErrNoWorkspace", err)
	}
	if v.Exists("/anything") {
		t.Fatal("zero View reports a file")
	}
}

func TestOSAliasesASymlinkedRoot(t *testing.T) {
	base := t.TempDir()
	real := filepath.Join(base, "real")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(real, "f"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(base, "alias")
	testutil.SymlinkOrSkip(t, real, alias)
	ws, err := workspace.OS(alias)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := filepath.EvalSymlinks(alias)
	if err != nil {
		t.Fatal(err)
	}
	v := workspace.NewView(ws)
	for _, p := range []string{filepath.Join(alias, "f"), filepath.Join(resolved, "f")} {
		if data, err := v.ReadFile(p); err != nil || string(data) != "x" {
			t.Fatalf("ReadFile(%s) = %q, %v", p, data, err)
		}
	}
}

func TestAroundRootsAtTheVCSTop(t *testing.T) {
	needGit(t)
	top := t.TempDir()
	runGit(t, top, "init", "-q", ".")
	sub := projectIn(t, top, "svc/api")
	runGit(t, top, "add", "svc/api/.ai-rulez/config.toml")
	ws, err := workspace.Around(sub)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := ws.Root(), top; got != want {
		t.Fatalf("Around root = %s, want %s", got, want)
	}
	bare := t.TempDir()
	ws, err = workspace.AroundBelow(t.Context(), gitutil.Git{}, bare, filepath.Dir(bare)) // a repository enclosing the temporary directory is not this test's
	if err != nil {
		t.Fatal(err)
	}
	if ws.Root() != bare {
		t.Fatalf("Around without a repository = %s, want %s", ws.Root(), bare)
	}
}

package catalogsite

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func site(files map[string]string) *Site {
	s := &Site{Files: map[string][]byte{}}
	for k, v := range files {
		s.Files[k] = []byte(v)
	}
	return s
}

func TestWrite_DirectoryRules(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T, dir string)
		clean   bool
		wantErr string
	}{
		{name: "missing directory is created", setup: func(t *testing.T, dir string) { require.NoError(t, os.RemoveAll(dir)) }},
		{name: "empty directory is accepted", setup: func(*testing.T, string) {}},
		{name: "marked directory is accepted", setup: func(t *testing.T, dir string) {
			require.NoError(t, os.WriteFile(filepath.Join(dir, MarkerFile), []byte("a.txt\n"), 0o644))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "other.txt"), []byte("x"), 0o644))
		}},
		{name: "non-empty directory without marker is refused", setup: func(t *testing.T, dir string) {
			require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("package x"), 0o644))
		}, wantErr: "no .ai-rulez-catalog marker"},
		{name: "clean without marker is refused", clean: true, setup: func(t *testing.T, dir string) {
			require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("package x"), 0o644))
		}, wantErr: "refusing to write and cleaning"},
		{name: "marker that is a directory is not a marker", setup: func(t *testing.T, dir string) {
			require.NoError(t, os.Mkdir(filepath.Join(dir, MarkerFile), 0o755))
		}, wantErr: "no .ai-rulez-catalog marker"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := filepath.Join(t.TempDir(), "out")
			require.NoError(t, os.Mkdir(dir, 0o755))
			tt.setup(t, dir)

			// Act
			res, err := Write(dir, site(map[string]string{"index.html": "x", "assets/a.css": "y"}), tt.clean)

			// Assert
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.NoFileExists(t, filepath.Join(dir, "index.html"))
				return
			}
			require.NoError(t, err)
			assert.Equal(t, 2, res.Written)
			assert.FileExists(t, filepath.Join(dir, "assets", "a.css"))
			assert.FileExists(t, filepath.Join(dir, MarkerFile))
		})
	}
}

func TestWrite_CleanRemovesOnlyWhatTheMarkerListed(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	_, err := Write(dir, site(map[string]string{"index.html": "1", "items/old.html": "1", "keep.html": "1"}), false)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "mine.txt"), []byte("user file"), 0o644))

	// Act
	res, err := Write(dir, site(map[string]string{"index.html": "2", "keep.html": "2"}), true)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, []string{"items/old.html"}, res.Removed)
	assert.NoFileExists(t, filepath.Join(dir, "items", "old.html"))
	assert.NoDirExists(t, filepath.Join(dir, "items"), "the emptied directory goes too")
	assert.FileExists(t, filepath.Join(dir, "mine.txt"))
	data, err := os.ReadFile(filepath.Join(dir, "index.html"))
	require.NoError(t, err)
	assert.Equal(t, "2", string(data))
}

func TestWrite_WithoutCleanKeepsStaleFilesListed(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	_, err := Write(dir, site(map[string]string{"a.html": "1", "b.html": "1"}), false)
	require.NoError(t, err)

	// Act
	_, err = Write(dir, site(map[string]string{"a.html": "2"}), false)
	require.NoError(t, err)
	res, err := Write(dir, site(map[string]string{"a.html": "3"}), true)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, []string{"b.html"}, res.Removed)
}

func TestWrite_HostileMarkerCannotRemoveOutsideTheDirectory(t *testing.T) {
	// Arrange
	parent := t.TempDir()
	victim := filepath.Join(parent, "victim.txt")
	require.NoError(t, os.WriteFile(victim, []byte("precious"), 0o644))
	dir := filepath.Join(parent, "out")
	require.NoError(t, os.Mkdir(dir, 0o755))
	marker := "../victim.txt\n/etc/hosts\na/../../victim.txt\n" + victim + "\n.ai-rulez-catalog\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, MarkerFile), []byte(marker), 0o644))

	// Act
	res, err := Write(dir, site(map[string]string{"index.html": "x"}), true)

	// Assert
	require.NoError(t, err)
	assert.Empty(t, res.Removed)
	assert.FileExists(t, victim)
}

func TestWrite_SymlinkCannotRedirectWritesOrRemovals(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	// Arrange
	parent := t.TempDir()
	outside := filepath.Join(parent, "outside")
	require.NoError(t, os.Mkdir(outside, 0o755))
	secret := filepath.Join(outside, "secret.txt")
	require.NoError(t, os.WriteFile(secret, []byte("precious"), 0o644))
	dir := filepath.Join(parent, "out")
	require.NoError(t, os.Mkdir(dir, 0o755))
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, "link")))
	require.NoError(t, os.WriteFile(filepath.Join(dir, MarkerFile), []byte("link/secret.txt\n"), 0o644))

	// Act
	_, err := Write(dir, site(map[string]string{"link/pwned.html": "x"}), true)

	// Assert
	require.Error(t, err)
	assert.NoFileExists(t, filepath.Join(outside, "pwned.html"))
	assert.FileExists(t, secret)
}

func TestWrite_IsByteStableAcrossRuns(t *testing.T) {
	// Arrange
	rendered, err := Render(hostileDoc(), Options{})
	require.NoError(t, err)
	a, b := t.TempDir(), t.TempDir()

	// Act
	_, errA := Write(a, rendered, false)
	_, errB := Write(b, rendered, false)

	// Assert
	require.NoError(t, errA)
	require.NoError(t, errB)
	for _, p := range rendered.Paths() {
		da, err := os.ReadFile(filepath.Join(a, filepath.FromSlash(p)))
		require.NoError(t, err)
		db, err := os.ReadFile(filepath.Join(b, filepath.FromSlash(p)))
		require.NoError(t, err)
		assert.Equal(t, da, db, p)
	}
	ma, _ := os.ReadFile(filepath.Join(a, MarkerFile))
	mb, _ := os.ReadFile(filepath.Join(b, MarkerFile))
	assert.Equal(t, ma, mb)
	assert.True(t, strings.Contains(string(ma), "catalog.json"))
}

func TestSegment(t *testing.T) {
	tests := []struct {
		in    string
		want  string // prefix; a hash suffix is expected when altered
		lossy bool
	}{
		{"deploy", "deploy", false},
		{"my_skill-1.2", "my_skill-1.2", false},
		{"Deploy", "deploy-", true},
		{"a/b", "a-b-", true},
		{"..", "x-", true},
		{"con", "con-", true},
		{"", "x-", true},
		{"<script>", "-script--", true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			// Act
			got := segment(tt.in)

			// Assert
			if tt.lossy {
				assert.True(t, strings.HasPrefix(got, tt.want), got)
				assert.Len(t, got, len(tt.want)+hashLen, got)
			} else {
				assert.Equal(t, tt.want, got)
			}
			assert.NotContains(t, got, "/")
			assert.NotContains(t, got, "..")
		})
	}
	assert.NotEqual(t, segment("Deploy"), segment("deploy-"), "different originals never share a name")
	assert.LessOrEqual(t, len(segment(strings.Repeat("a", 1_000_000))), maxSlugLen+1+hashLen)
}

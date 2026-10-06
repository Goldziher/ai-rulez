package workspace

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
)

// gitModeSymlink and gitModeSubmodule are the tree modes git records for a
// symlink and for a submodule commit.
const (
	gitModeSymlink   = "120000"
	gitModeSubmodule = "160000"
	gitModeExec      = "100755"
)

// Snapshot is a Workspace over the tree of one commit.
type Snapshot interface {
	Workspace
	// Commit is the full id of the commit the snapshot reads.
	Commit() string
}

// GitSnapshot returns a read-only Workspace over the tree of rev in the
// repository at repoDir, without checking anything out. Git runs through r (nil:
// real git); the tree is listed once and file contents are read on demand with
// `git cat-file`, using ctx, so a snapshot belongs to one request.
//
// Modes come from the tree entry: a symlink reads as a symlink whose target is
// resolved inside the tree only, a submodule appears as an empty directory.
// Machine-local files (.ai-rulez/local, config.local.toml) are not in a commit,
// so a snapshot behaves like a load with --no-local.
//
// With paths (slash paths relative to the repository root, literal), only those
// files and directories are listed: reading one file of a large repository does
// not list the whole tree.
func GitSnapshot(ctx context.Context, repoDir, rev string, r runner.Runner, paths ...string) (Snapshot, error) {
	rev = strings.TrimSpace(rev)
	if err := gitutil.CheckArg("revision", rev); err != nil || rev == "" {
		return nil, oops.With("rev", rev).Errorf("invalid git revision %q", rev)
	}
	g := gitutil.New(r)
	res := g.Exec(ctx, repoDir, nil, "rev-parse", "--verify", "--quiet", rev+"^{commit}")
	if err := gitutil.ResultErr(res); err != nil {
		return nil, oops.With("rev", rev, "dir", repoDir).Wrapf(err, "resolve revision")
	}
	commit := strings.TrimSpace(string(res.Stdout))
	args := []string{"ls-tree", "-r", "-z", "--long", "--full-tree", commit}
	if len(paths) > 0 {
		args = append(args, "--")
		args = append(args, paths...)
	}
	res = g.Exec(ctx, repoDir, nil, args...)
	if err := gitutil.ResultErr(res); err != nil {
		return nil, oops.With("rev", rev, "commit", commit).Wrapf(err, "list the tree")
	}
	s := &snapshot{ctx: ctx, git: g, dir: repoDir, commit: commit, nodes: map[string]*node{".": {mode: fs.ModeDir | 0o755}}, blobs: map[string][]byte{}}
	for _, record := range bytes.Split(res.Stdout, []byte{0}) {
		if len(record) == 0 {
			continue
		}
		if err := s.add(string(record)); err != nil {
			return nil, err
		}
	}
	for _, n := range s.nodes {
		sort.Strings(n.children)
	}
	return s, nil
}

type node struct {
	mode     fs.FileMode
	oid      string
	size     int64
	children []string // names, for a directory
}

type snapshot struct {
	ctx    context.Context
	git    gitutil.Git
	dir    string
	commit string
	nodes  map[string]*node

	mu    sync.Mutex
	blobs map[string][]byte
}

// add records one `git ls-tree --long` entry: "<mode> <type> <oid> <size>\t<path>".
func (s *snapshot) add(record string) error {
	meta, name, ok := strings.Cut(record, "\t")
	fields := strings.Fields(meta)
	if !ok || len(fields) != 4 || !fs.ValidPath(name) {
		return oops.With("entry", record).Errorf("unexpected git tree entry")
	}
	mode, oid := fields[0], fields[2]
	size, _ := strconv.ParseInt(fields[3], 10, 64) //nolint:errcheck // "-" for a submodule
	n := &node{oid: oid, size: size}
	switch mode {
	case gitModeSubmodule:
		n.mode = fs.ModeDir | 0o755 // listed, never expanded
		n.oid = ""
	case gitModeSymlink:
		n.mode = fs.ModeSymlink | 0o777
	case gitModeExec:
		n.mode = 0o755
	default:
		n.mode = 0o644
	}
	if _, dup := s.nodes[name]; dup {
		return nil
	}
	s.nodes[name] = n
	// Register the entry with its parent, and with every ancestor that is new: an
	// existing directory already hangs off its own parent, so each name is
	// appended exactly once and no membership scan is needed.
	for child := name; child != "."; {
		parent := path.Dir(child)
		p, exists := s.nodes[parent]
		if !exists {
			p = &node{mode: fs.ModeDir | 0o755}
			s.nodes[parent] = p
		}
		p.children = append(p.children, path.Base(child))
		if exists {
			break
		}
		child = parent
	}
	return nil
}

func (s *snapshot) Commit() string { return s.commit }
func (s *snapshot) Root() string   { return s.dir }

type info struct {
	name string
	n    *node
}

func (i info) Name() string       { return i.name }
func (i info) Size() int64        { return i.n.size }
func (i info) Mode() fs.FileMode  { return i.n.mode }
func (i info) ModTime() time.Time { return time.Time{} }
func (i info) IsDir() bool        { return i.n.mode.IsDir() }
func (i info) Sys() any           { return nil }

func notExist(op, name string) error {
	return &fs.PathError{Op: op, Path: name, Err: fs.ErrNotExist}
}

func (s *snapshot) lookup(op, name string) (*node, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: op, Path: name, Err: fs.ErrInvalid}
	}
	n, ok := s.nodes[name]
	if !ok {
		return nil, notExist(op, name)
	}
	return n, nil
}

// follow resolves the symlinks of name inside the tree and returns the entry
// it names.
func (s *snapshot) follow(op, name string) (string, *node, error) {
	if !fs.ValidPath(name) {
		return "", nil, &fs.PathError{Op: op, Path: name, Err: fs.ErrInvalid}
	}
	resolved, err := Resolve(s, name)
	if err != nil {
		return "", nil, withName(err, op, name)
	}
	n, err := s.lookup(op, resolved)
	return resolved, n, err
}

func withName(err error, op, name string) error {
	if pe, ok := err.(*fs.PathError); ok { //nolint:errorlint // only a direct PathError is renamed
		return &fs.PathError{Op: op, Path: name, Err: pe.Err}
	}
	return &fs.PathError{Op: op, Path: name, Err: err}
}

func (s *snapshot) Lstat(name string) (fs.FileInfo, error) {
	n, err := s.lookup("lstat", name)
	if err != nil {
		return nil, err
	}
	return info{path.Base(name), n}, nil
}

func (s *snapshot) Stat(name string) (fs.FileInfo, error) {
	_, n, err := s.follow("stat", name)
	if err != nil {
		return nil, err
	}
	return info{path.Base(name), n}, nil
}

func (s *snapshot) ReadLink(name string) (string, error) {
	n, err := s.lookup("readlink", name)
	if err != nil {
		return "", err
	}
	if n.mode&fs.ModeSymlink == 0 {
		return "", &fs.PathError{Op: "readlink", Path: name, Err: fs.ErrInvalid}
	}
	data, err := s.blob(n.oid)
	if err != nil {
		return "", withName(err, "readlink", name)
	}
	return string(data), nil
}

func (s *snapshot) ReadFile(name string) ([]byte, error) {
	_, n, err := s.follow("open", name)
	if err != nil {
		return nil, err
	}
	if n.mode.IsDir() {
		return nil, &fs.PathError{Op: "read", Path: name, Err: fs.ErrInvalid}
	}
	data, err := s.blob(n.oid)
	if err != nil {
		return nil, withName(err, "open", name)
	}
	return append([]byte(nil), data...), nil
}

func (s *snapshot) ReadDir(name string) ([]fs.DirEntry, error) {
	_, n, err := s.follow("open", name)
	if err != nil {
		return nil, err
	}
	if !n.mode.IsDir() {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrInvalid}
	}
	dir, _ := Resolve(s, name) //nolint:errcheck // follow succeeded
	entries := make([]fs.DirEntry, 0, len(n.children))
	for _, child := range n.children {
		cn := s.nodes[path.Join(dir, child)]
		if cn == nil {
			continue
		}
		entries = append(entries, fs.FileInfoToDirEntry(info{child, cn}))
	}
	return entries, nil
}

func (s *snapshot) Open(name string) (fs.File, error) {
	resolved, n, err := s.follow("open", name)
	if err != nil {
		return nil, err
	}
	if n.mode.IsDir() {
		entries, err := s.ReadDir(name)
		if err != nil {
			return nil, err
		}
		return &dirFile{info: info{path.Base(resolved), n}, entries: entries}, nil
	}
	data, err := s.blob(n.oid)
	if err != nil {
		return nil, withName(err, "open", name)
	}
	return &memFile{info: info{path.Base(resolved), n}, Reader: bytes.NewReader(data)}, nil
}

// blob reads one object, once.
func (s *snapshot) blob(oid string) ([]byte, error) {
	s.mu.Lock()
	data, ok := s.blobs[oid]
	s.mu.Unlock()
	if ok {
		return data, nil
	}
	res := s.git.Exec(s.ctx, s.dir, nil, "cat-file", "blob", oid)
	if err := gitutil.ResultErr(res); err != nil {
		return nil, oops.With("oid", oid, "commit", s.commit).Wrapf(err, "read object")
	}
	if res.StdoutTruncated {
		return nil, oops.With("oid", oid, "commit", s.commit).Errorf("object is larger than the read cap")
	}
	s.mu.Lock()
	s.blobs[oid] = res.Stdout
	s.mu.Unlock()
	return res.Stdout, nil
}

type memFile struct {
	info info
	*bytes.Reader
}

func (f *memFile) Stat() (fs.FileInfo, error) { return f.info, nil }
func (f *memFile) Close() error               { return nil }

type dirFile struct {
	info    info
	entries []fs.DirEntry
	pos     int
}

func (d *dirFile) Stat() (fs.FileInfo, error) { return d.info, nil }
func (d *dirFile) Close() error               { return nil }
func (d *dirFile) Read([]byte) (int, error) {
	return 0, &fs.PathError{Op: "read", Path: d.info.name, Err: fs.ErrInvalid}
}

func (d *dirFile) ReadDir(count int) ([]fs.DirEntry, error) {
	rest := d.entries[d.pos:]
	if count <= 0 {
		d.pos = len(d.entries)
		return rest, nil
	}
	if len(rest) == 0 {
		return nil, io.EOF
	}
	if count > len(rest) {
		count = len(rest)
	}
	d.pos += count
	return rest[:count], nil
}

// MaxBaseFileBytes caps what ReadFileAt reads: the same per-file cap content
// loading applies, so a base revision cannot be used to read an unbounded blob.
const MaxBaseFileBytes = 8 << 20

// maxSymlinkHops bounds how many symlinks ReadFileAt follows.
const maxSymlinkHops = 40

// ReadFileAt returns the file rel (relative to the repository root) as it was at
// rev, the way `git show rev:rel` does, without listing the rest of the tree.
// found is false, with a nil error, when the file does not exist at rev. An
// unknown revision, git failing to run, a blob over MaxBaseFileBytes or a
// symlink that leaves the tree is an error: a caller never mistakes those for an
// absent file. A symlink entry (also in an ancestor directory) is followed inside
// the tree, like a snapshot does.
func ReadFileAt(ctx context.Context, repoDir, rev, rel string, r runner.Runner) (content []byte, found bool, err error) {
	rev = strings.TrimSpace(rev)
	if err := gitutil.CheckArg("revision", rev); err != nil || rev == "" {
		return nil, false, oops.With("rev", rev).Errorf("invalid git revision %q", rev)
	}
	g := gitutil.New(r)
	res := g.Exec(ctx, repoDir, nil, "rev-parse", "--verify", "--quiet", rev+"^{commit}")
	if err := gitutil.ResultErr(res); err != nil {
		return nil, false, oops.With("rev", rev, "dir", repoDir).Wrapf(err, "resolve revision")
	}
	commit := strings.TrimSpace(string(res.Stdout))
	for range maxSymlinkHops {
		if !fs.ValidPath(rel) || rel == "." {
			return nil, false, oops.With("path", rel).Errorf("invalid path in revision %s", rev)
		}
		next, data, done, err := stepRead(ctx, g, repoDir, commit, rel)
		if err != nil {
			return nil, false, err
		}
		if done {
			return data, data != nil, nil
		}
		rel = next
	}
	return nil, false, oops.With("path", rel, "rev", rev).Errorf("too many symbolic links")
}

type treeEntry struct {
	mode, kind, oid string
	size            int64
}

// lookupEntry lists the tree entry of one path (a directory is one entry, not its
// contents) at commit; ok is false when the path is absent.
func lookupEntry(ctx context.Context, g gitutil.Git, dir, commit, name string) (e treeEntry, ok bool, err error) {
	res := g.Exec(ctx, dir, nil, "ls-tree", "-z", "--long", "--full-tree", commit, "--", name)
	if err := gitutil.ResultErr(res); err != nil {
		return treeEntry{}, false, oops.With("commit", commit, "path", name).Wrapf(err, "list the tree")
	}
	for _, record := range bytes.Split(res.Stdout, []byte{0}) {
		if len(record) == 0 {
			continue
		}
		meta, got, cut := strings.Cut(string(record), "\t")
		fields := strings.Fields(meta)
		if !cut || len(fields) != 4 {
			return treeEntry{}, false, oops.With("entry", string(record)).Errorf("unexpected git tree entry")
		}
		if got != name {
			continue
		}
		size, _ := strconv.ParseInt(fields[3], 10, 64) //nolint:errcheck // "-" for a tree
		return treeEntry{mode: fields[0], kind: fields[1], oid: fields[2], size: size}, true, nil
	}
	return treeEntry{}, false, nil
}

// stepRead resolves one step of ReadFileAt: the content when rel is a regular
// file (done), the rewritten path when a prefix is a symlink, or "absent" (done,
// nil data) when a prefix is missing or a directory.
func stepRead(ctx context.Context, g gitutil.Git, dir, commit, rel string) (next string, data []byte, done bool, err error) {
	parts := strings.Split(rel, "/")
	for i := range parts {
		prefix := strings.Join(parts[:i+1], "/")
		e, ok, err := lookupEntry(ctx, g, dir, commit, prefix)
		if err != nil {
			return "", nil, false, err
		}
		if !ok {
			return "", nil, true, nil
		}
		last := i == len(parts)-1
		switch {
		case e.mode == gitModeSymlink:
			target, err := readBlob(ctx, g, dir, commit, e, 4096)
			if err != nil {
				return "", nil, false, err
			}
			joined := path.Join(path.Dir(prefix), string(target))
			if path.IsAbs(string(target)) || !fs.ValidPath(joined) {
				return "", nil, false, oops.With("path", prefix, "target", string(target)).Errorf("symlink leaves the tree")
			}
			if rest := strings.Join(parts[i+1:], "/"); rest != "" {
				joined = path.Join(joined, rest)
			}
			return joined, nil, false, nil
		case last && e.kind == "blob":
			content, err := readBlob(ctx, g, dir, commit, e, MaxBaseFileBytes)
			if err != nil {
				return "", nil, false, err
			}
			if content == nil {
				content = []byte{}
			}
			return "", content, true, nil
		case last || e.kind != "tree":
			return "", nil, true, nil // a directory or submodule is not a file
		}
	}
	return "", nil, true, nil
}

// readBlob reads the object of e, refusing one larger than limit and output the
// runner cut short.
func readBlob(ctx context.Context, g gitutil.Git, dir, commit string, e treeEntry, limit int64) ([]byte, error) {
	if e.size > limit {
		return nil, oops.With("oid", e.oid, "size", e.size, "limit", limit).Errorf("blob is larger than %d bytes", limit)
	}
	res := g.Exec(ctx, dir, nil, "cat-file", "blob", e.oid)
	if err := gitutil.ResultErr(res); err != nil {
		return nil, oops.With("oid", e.oid, "commit", commit).Wrapf(err, "read object")
	}
	if res.StdoutTruncated {
		return nil, oops.With("oid", e.oid, "commit", commit).Errorf("object output was truncated")
	}
	return res.Stdout, nil
}

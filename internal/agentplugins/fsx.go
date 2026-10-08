package agentplugins

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"strings"
)

// errEscape marks a path that resolves outside the plugin root (§4.1).
var errEscape = errors.New("path resolves outside the plugin root")

const maxLinkHops = 40

// resolve returns the path name designates inside fsys with every symbolic
// link followed, component by component, or errEscape when it leaves the root.
// An absolute link target is treated as leaving the root: containment cannot
// be proven against an abstract file system. Missing components are kept as
// they are. A file system without symlink support resolves to name itself.
func resolve(fsys fs.FS, name string) (string, error) {
	rl, ok := fsys.(fs.ReadLinkFS)
	if !ok {
		return name, nil
	}
	pending := strings.Split(name, "/")
	var done []string
	hops := 0
	for len(pending) > 0 {
		seg := pending[0]
		pending = pending[1:]
		switch seg {
		case "", ".":
			continue
		case "..":
			if len(done) == 0 {
				return "", errEscape
			}
			done = done[:len(done)-1]
			continue
		}
		cur := path.Join(path.Join(done...), seg)
		info, err := rl.Lstat(cur)
		if errors.Is(err, fs.ErrNotExist) || (err == nil && info.Mode()&fs.ModeSymlink == 0) {
			done = append(done, seg)
			continue
		}
		if err != nil {
			return "", err
		}
		hops++
		if hops > maxLinkHops {
			return "", fmt.Errorf("too many levels of symbolic links at %s", cur)
		}
		target, err := readContainedLink(rl, cur)
		if err != nil {
			return "", err
		}
		pending = append(strings.Split(target, "/"), pending...)
	}
	if len(done) == 0 {
		return ".", nil
	}
	return path.Join(done...), nil
}

// readContainedLink returns the target of the link at name, refusing an
// absolute target.
func readContainedLink(rl fs.ReadLinkFS, name string) (string, error) {
	target, err := rl.ReadLink(name)
	if err != nil {
		return "", err
	}
	if target == "" || path.IsAbs(target) || filepath.IsAbs(target) || strings.Contains(target, `\`) {
		return "", errEscape
	}
	return target, nil
}

type fileState int

const (
	fileOK fileState = iota
	fileAbsent
	fileEscapes
	fileNotRegular
	fileUnreadable
	fileTooLarge
)

// maxFileBytes caps one file read from a plugin, like the native importers
// (internal/importer/fsread.go). maxTotalBytes caps everything one Import reads;
// it is a variable so tests can lower it.
const maxFileBytes = 2 << 20

var maxTotalBytes int64 = 64 << 20

var errTooLarge = fmt.Errorf("file is larger than the %d MiB limit", maxFileBytes>>20)

// statResolved resolves name and reports what it designates.
func statResolved(fsys fs.FS, name string) (string, fileState, error) {
	res, err := resolve(fsys, name)
	if errors.Is(err, errEscape) {
		return "", fileEscapes, err
	}
	if err != nil {
		return "", fileUnreadable, err
	}
	info, err := fs.Stat(fsys, res)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return res, fileAbsent, err
	case err != nil:
		return res, fileUnreadable, err
	case !info.Mode().IsRegular():
		return res, fileNotRegular, nil
	}
	return res, fileOK, nil
}

// readResolved reads the regular file name designates inside fsys.
func readResolved(fsys fs.FS, name string) ([]byte, fileState, error) {
	res, state, err := statResolved(fsys, name)
	if state != fileOK {
		return nil, state, err
	}
	if info, err := fs.Stat(fsys, res); err == nil && info.Size() > maxFileBytes {
		return nil, fileTooLarge, errTooLarge
	}
	data, err := fs.ReadFile(fsys, res)
	if err != nil {
		return nil, fileUnreadable, err
	}
	return data, fileOK, nil
}

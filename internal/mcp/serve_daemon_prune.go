//go:build !windows

package mcp

import (
	"os"
	"path/filepath"
	"time"
)

// staleDaemonDirAge is how long a daemon directory must sit unused before a
// starting daemon removes it.
const staleDaemonDirAge = 7 * 24 * time.Hour

// pruneStaleDaemonDirsBeside prunes the siblings of the daemon directory that
// holds socket, judging their age at now.
func pruneStaleDaemonDirsBeside(socket string, now time.Time) {
	dir := filepath.Dir(socket)
	pruneStaleDaemonDirs(filepath.Dir(dir), dir, staleDaemonDirAge, now)
}

// pruneStaleDaemonDirs removes the daemon directories beside keep that nothing
// has touched for maxAge and that hold no live daemon: a daemon removes its
// socket on exit but leaves the directory and its log, so without this one
// directory per repository and version would stay for good. A directory with
// anything but a log and a dead socket in it is left alone.
func pruneStaleDaemonDirs(parent, keep string, maxAge time.Duration, now time.Time) {
	entries, err := os.ReadDir(parent)
	if err != nil {
		return
	}
	for _, e := range entries {
		dir := filepath.Join(parent, e.Name())
		if !e.IsDir() || dir == keep {
			continue
		}
		if info, err := os.Stat(dir); err != nil || now.Sub(info.ModTime()) < maxAge {
			continue
		}
		if daemonDirRemovable(dir) {
			_ = os.RemoveAll(dir) //nolint:errcheck // best-effort housekeeping
		}
	}
}

func daemonDirRemovable(dir string) bool {
	files, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, f := range files {
		switch f.Name() {
		case "daemon.log":
		case daemonSocketName, shortSocketName:
			if live, _ := socketLive(filepath.Join(dir, f.Name())); live { //nolint:errcheck // a failed probe means not live
				return false
			}
		default:
			return false
		}
	}
	return true
}

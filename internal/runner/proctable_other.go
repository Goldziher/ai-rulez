//go:build !windows && !darwin && !linux

package runner

import (
	"errors"
	"os"
)

// processTable is not implemented here: only the process group is killed.
func processTable() ([]procEntry, error) {
	return nil, errors.New("process table not supported on this platform")
}

// processEnv is not implemented here (there is no process table to search).
func processEnv(int) ([]string, error) {
	return nil, errors.New("process environment not supported on this platform")
}

// markerID is not implemented here: the run is not marked.
func markerID(*os.File) (uint64, error) {
	return 0, errors.New("marker pipe not supported on this platform")
}

// holdsMarker is not implemented here.
func holdsMarker(int, uint64) bool { return false }

// processStart is not implemented here.
func processStart(int) (int64, bool) { return 0, false }

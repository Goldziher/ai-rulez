//go:build !windows && !darwin && !linux

package runner

import "errors"

// processTable is not implemented here: only the process group is killed.
func processTable() ([]procEntry, error) {
	return nil, errors.New("process table not supported on this platform")
}

// processEnv is not implemented here (there is no process table to search).
func processEnv(int) ([]string, error) {
	return nil, errors.New("process environment not supported on this platform")
}

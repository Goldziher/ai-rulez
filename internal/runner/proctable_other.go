//go:build !windows && !darwin && !linux

package runner

import "errors"

// processTable is not implemented here: only the process group is killed.
func processTable() ([]procEntry, error) {
	return nil, errors.New("process table not supported on this platform")
}

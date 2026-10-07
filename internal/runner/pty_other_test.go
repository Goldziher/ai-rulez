//go:build !windows && !darwin && !linux

package runner

import (
	"errors"
	"os"
)

func openPTY() (*os.File, string, error) {
	return nil, "", errors.New("pseudo-terminals are only opened on darwin and linux")
}

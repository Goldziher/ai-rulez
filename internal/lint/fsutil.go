package lint

import (
	"errors"
	"os"
)

// maxReadBytes bounds the files the config-shape checks read.
const maxReadBytes = 2 << 20

var errTooLarge = errors.New("file too large")

func readSmallFile(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err //nolint:wrapcheck // callers only test for failure
	}
	if info.Size() > maxReadBytes {
		return nil, errTooLarge
	}
	return os.ReadFile(path) //nolint:wrapcheck // callers only test for failure
}

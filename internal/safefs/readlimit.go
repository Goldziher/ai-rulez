package safefs

import (
	"errors"
	"fmt"
	"io"
	"os"
)

// ErrTooLarge is returned by ReadLimited when the input is longer than the limit.
var ErrTooLarge = errors.New("input exceeds the size limit")

// ReadLimited reads r to the end, at most max bytes. Input longer than max is an
// error wrapping ErrTooLarge, never a silently truncated prefix: a cut JSON
// document, hash or generated file is worse than a clear refusal.
func ReadLimited(r io.Reader, maxBytes int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return nil, err //nolint:wrapcheck // the reader's own error is the cause
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("%w (limit %d bytes)", ErrTooLarge, maxBytes)
	}
	return data, nil
}

// ReadFileLimited opens the file at path and reads it with ReadLimited. Like
// os.ReadFile it follows a symlink, so callers that must refuse links check the
// path first (see ReadRegular). The error names the path and wraps ErrTooLarge
// for a file longer than maxBytes.
func ReadFileLimited(path string, maxBytes int64) ([]byte, error) {
	f, err := os.Open(path) //nolint:gosec // the caller names the path and bounds the read
	if err != nil {
		return nil, err //nolint:wrapcheck // callers test os.ErrNotExist
	}
	defer f.Close() //nolint:errcheck // read-only
	data, err := ReadLimited(f, maxBytes)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return data, nil
}

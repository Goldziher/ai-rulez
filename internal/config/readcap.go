package config

import (
	"io"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/workspace"
)

// maxContentFileBytes caps one file read from repository content (a config
// file, a rule, a skill or one of its resources). A repository is untrusted
// input: without a cap a single huge file, or a link to /dev/zero, exhausts
// memory. The cap is fixed; a larger file is an error, never a truncation.
const maxContentFileBytes = 8 << 20

// readCapped reads path in full, failing when it is larger than maxContentFileBytes.
func readCapped(v workspace.View, path string) ([]byte, error) {
	f, err := v.Open(path) //nolint:gosec // callers pass content paths already checked by the symlink policy
	if err != nil {
		return nil, err //nolint:wrapcheck // callers test os.ErrNotExist and add context
	}
	defer f.Close() //nolint:errcheck // read-only
	data, err := io.ReadAll(io.LimitReader(f, maxContentFileBytes+1))
	if err != nil {
		return nil, err //nolint:wrapcheck // callers add context
	}
	if len(data) > maxContentFileBytes {
		return nil, oops.With("path", path).Errorf("%s is larger than the %d MiB limit for repository content", path, maxContentFileBytes>>20)
	}
	return data, nil
}

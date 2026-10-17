package commands

import (
	"context"
	"errors"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// discoverProjectRoot returns the directory to load the project from: the
// nearest directory at or above the working directory that holds a
// configuration in any layout. It is "." when that is the working directory.
// A legacy configuration is not a walk-up result: the loader of "." reports
// it. A missing configuration is returned as the search error, which names
// where it looked.
func discoverProjectRoot(ctx context.Context) (string, error) {
	cwd := workingDir()
	if cwd == "" {
		return ".", nil
	}
	root, err := config.DiscoverProjectRoot(ctx, cwd)
	if err != nil {
		if errors.Is(err, config.ErrNoConfig) {
			return "", err //nolint:wrapcheck // already contextual
		}
		return ".", nil
	}
	if config.SameDir(root, cwd) {
		return ".", nil
	}
	return root, nil
}

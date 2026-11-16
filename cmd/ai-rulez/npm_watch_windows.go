//go:build windows

package main

import (
	"os"

	"github.com/Goldziher/ai-rulez/v5/internal/npmwatch"
	"github.com/samber/oops"
)

func watchNPMParent() error {
	parent, ok := os.LookupEnv(npmwatch.ParentEnvironment)
	if !ok {
		return nil
	}
	if err := os.Unsetenv(npmwatch.ParentEnvironment); err != nil {
		return oops.Wrapf(err, "clear npm wrapper process marker")
	}
	return npmwatch.Start(parent, os.Getppid())
}

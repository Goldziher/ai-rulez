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
	inJob, err := npmwatch.Start(parent, os.Getppid())
	if err != nil || !inJob {
		return err
	}
	if err := os.Setenv(npmwatch.ParentEnvironment, npmwatch.JobMarker(os.Getpid())); err != nil {
		return oops.Wrapf(err, "mark CLI as enrolled in the npm wrapper job")
	}
	return nil
}

package policy

import "runtime"

func hostOS() string { return runtime.GOOS }

// managedRoot moves the managed policy location for a binary built by a test that
// cannot write the real one without root:
//
//	go build -ldflags "-X github.com/Goldziher/ai-rulez/v5/internal/policy.managedRoot=<dir>"
//
// The managed policy is then <dir>/ai-rulez/policy.toml. It is a link-time
// variable of that one binary, deliberately not an environment variable or a
// flag: anything a repository or a shell can set could replace the managed
// anchor with an empty file, which is exactly what the anchor exists to prevent.
// A release binary leaves it empty.
var managedRoot string

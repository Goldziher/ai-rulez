package policy

import "runtime"

func hostOS() string { return runtime.GOOS }

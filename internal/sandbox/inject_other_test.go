//go:build !darwin

package sandbox

// injectProbe is only exercised on macOS, where the sandbox-exec profile is tested.
func injectProbe(string) string { return "unsupported" }

//go:build windows

package commands

// setUmask is a no-op on Windows, where the umask tests skip.
func setUmask(int) int { return 0 }

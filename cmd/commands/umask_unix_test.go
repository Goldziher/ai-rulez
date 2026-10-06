//go:build !windows

package commands

import "syscall"

// setUmask swaps the process umask and returns the previous one.
func setUmask(mask int) int { return syscall.Umask(mask) }

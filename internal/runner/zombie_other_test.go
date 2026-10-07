//go:build !windows && !darwin && !linux

package runner

func isZombie(int) bool { return false }

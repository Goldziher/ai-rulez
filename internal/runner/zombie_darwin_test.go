package runner

import "golang.org/x/sys/unix"

// isZombie reports whether pid has exited and waits to be reaped.
func isZombie(pid int) bool {
	kp, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	return err == nil && kp.Proc.P_stat == 5 // SZOMB
}

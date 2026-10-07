package runner

import "golang.org/x/sys/unix"

// processTable lists every process of the system (see procEntry). The session
// is left unset: the kernel reports it as an address that a later session may
// reuse, not as an id.
func processTable() ([]procEntry, error) {
	kps, err := unix.SysctlKinfoProcSlice("kern.proc.all")
	if err != nil {
		return nil, err //nolint:wrapcheck // the caller falls back to the group kill
	}
	out := make([]procEntry, 0, len(kps))
	for i := range kps {
		kp := &kps[i]
		out = append(out, procEntry{
			pid:   int(kp.Proc.P_pid),
			ppid:  int(kp.Eproc.Ppid),
			pgid:  int(kp.Eproc.Pgid),
			uid:   int(kp.Eproc.Ucred.Uid),
			start: kp.Proc.P_starttime.Sec*1_000_000 + int64(kp.Proc.P_starttime.Usec),
		})
	}
	return out, nil
}

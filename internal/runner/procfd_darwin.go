package runner

import (
	"encoding/binary"
	"errors"
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

// proc_info(2) call numbers and the sizes and offsets of the structures it
// fills (sys/proc_info.h).
const (
	procInfoCallPidinfo   = 2
	procInfoCallPidfdinfo = 3
	procPidListFDs        = 1 // PROC_PIDLISTFDS: proc_fdinfo[]
	procPidFDPipeInfo     = 6 // PROC_PIDFDPIPEINFO: pipe_fdinfo
	proxFDTypePipe        = 6 // PROX_FDTYPE_PIPE
	procFDInfoSize        = 8 // sizeof(struct proc_fdinfo)
	pipeFDInfoSize        = 184
	pipeHandleOff         = 160 // offsetof(pipe_fdinfo, pipeinfo.pipe_handle)
	pipePeerHandleOff     = 168 // offsetof(pipe_fdinfo, pipeinfo.pipe_peerhandle)
	maxScannedFDs         = 4096
)

// procInfo is the proc_info(2) system call behind libproc's proc_pidinfo and
// proc_pidfdinfo. libproc is reachable from Go only through cgo, which the
// release builds do not use, so the call is made directly; its number and
// layouts have not changed since Mac OS X 10.5. A failure only means the
// marker check finds nothing.
func procInfo(call, pid, flavor int, arg uint64, buf []byte) (int, error) {
	//nolint:staticcheck,gosec // SA1019: no libSystem wrapper without cgo (see above); the ints are small and non-negative
	r, _, errno := unix.Syscall6(unix.SYS_PROC_INFO, uintptr(call), uintptr(pid), uintptr(flavor),
		uintptr(arg), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if errno != 0 {
		return 0, errno
	}
	return int(r), nil //nolint:gosec // a byte count no larger than len(buf)
}

// pipeHandles returns the kernel handle of the pipe end fd of pid and of its peer end.
func pipeHandles(pid, fd int) (self, peer uint64, err error) {
	buf := make([]byte, pipeFDInfoSize)
	n, err := procInfo(procInfoCallPidfdinfo, pid, procPidFDPipeInfo, uint64(fd), buf) //nolint:gosec // fd is non-negative
	if err != nil {
		return 0, 0, err
	}
	if n < pipePeerHandleOff+8 {
		return 0, 0, errors.New("proc_info: short pipe info")
	}
	return binary.LittleEndian.Uint64(buf[pipeHandleOff:]), binary.LittleEndian.Uint64(buf[pipePeerHandleOff:]), nil
}

// markerID identifies the run's marker pipe by the handle of the read end this
// process holds; a process holding the write end sees it as its peer.
func markerID(r *os.File) (uint64, error) {
	self, _, err := pipeHandles(os.Getpid(), int(r.Fd())) //nolint:gosec // an open descriptor fits in int
	return self, err
}

// holdsMarker reports whether pid has a descriptor on the write end of the
// marker pipe identified by id.
func holdsMarker(pid int, id uint64) bool {
	buf := make([]byte, procFDInfoSize*maxScannedFDs)
	n, err := procInfo(procInfoCallPidinfo, pid, procPidListFDs, 0, buf)
	if err != nil {
		return false
	}
	for off := 0; off+procFDInfoSize <= n; off += procFDInfoSize {
		fd := int(int32(binary.LittleEndian.Uint32(buf[off:]))) //nolint:gosec // the kernel's int32
		if binary.LittleEndian.Uint32(buf[off+4:]) != proxFDTypePipe {
			continue
		}
		if _, peer, err := pipeHandles(pid, fd); err == nil && peer == id {
			return true
		}
	}
	return false
}

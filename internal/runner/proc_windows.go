//go:build windows

package runner

import (
	"os/exec"
	"unsafe"

	"golang.org/x/sys/windows"
)

func isWindows() bool { return true }

// procTree is a Job Object holding the child and everything it spawns, set to
// kill its members when the last handle closes.
type procTree struct{ job windows.Handle }

// configure creates the job and routes cancellation through it. If the job
// cannot be created the run still works and falls back to killing the child only.
func configure(cmd *exec.Cmd) *procTree {
	t := &procTree{}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return t
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil { //nolint:gosec // the API takes the struct by address
		_ = windows.CloseHandle(job) //nolint:errcheck // nothing to recover
		return t
	}
	t.job = job
	cmd.Cancel = func() error {
		if t.job != 0 {
			_ = windows.TerminateJobObject(t.job, 1) //nolint:errcheck // fall through to the direct kill
		}
		if cmd.Process == nil {
			return nil
		}
		return cmd.Process.Kill() //nolint:wrapcheck // the process may already be gone
	}
	return t
}

// shortLived is a no-op: the job object tracks the tree without polling.
func (t *procTree) shortLived() {}

// attach assigns the started child to the job. A grandchild spawned in the
// instant before this call escapes the job; that window is accepted.
func (t *procTree) attach(cmd *exec.Cmd) {
	if t.job == 0 || cmd.Process == nil {
		return
	}
	h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid)) //nolint:gosec // a Windows pid is a DWORD
	if err != nil {
		return
	}
	defer windows.CloseHandle(h)                   //nolint:errcheck // best effort
	_ = windows.AssignProcessToJobObject(t.job, h) //nolint:errcheck // falls back to killing the child only
}

func (t *procTree) kill(*exec.Cmd) {
	if t.job != 0 {
		_ = windows.TerminateJobObject(t.job, 1) //nolint:errcheck // nothing left to kill is fine
	}
}

func (t *procTree) close() {
	if t.job != 0 {
		_ = windows.CloseHandle(t.job) //nolint:errcheck // closing kills any remaining member
		t.job = 0
	}
}

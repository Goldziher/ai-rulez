//go:build windows

package evals

import (
	"os/exec"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// jobs maps a started command to the Job Object that holds it and its children.
var jobs sync.Map // *exec.Cmd -> windows.Handle

// killTreeOnCancel routes cancellation through a Job Object (created by runTree)
// so the whole tree dies, not just the direct child.
func killTreeOnCancel(cmd *exec.Cmd) {
	cmd.Cancel = func() error {
		killTree(cmd)
		if cmd.Process == nil {
			return nil
		}
		return cmd.Process.Kill() //nolint:wrapcheck // the process may already be gone
	}
	cmd.WaitDelay = 2 * time.Second
}

// runTree starts cmd, assigns it to a kill-on-close Job Object and waits. If the
// job cannot be created the run still works and only the child is killed. A
// grandchild spawned before the assignment escapes the job; that window is accepted.
func runTree(cmd *exec.Cmd) error {
	job := newKillOnCloseJob()
	if err := cmd.Start(); err != nil {
		closeJob(job)
		return err //nolint:wrapcheck // the caller adds context
	}
	if job != 0 {
		jobs.Store(cmd, job)
		defer func() {
			jobs.Delete(cmd)
			closeJob(job) // closing kills any remaining member
		}()
		if h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid)); err == nil {
			_ = windows.AssignProcessToJobObject(job, h) //nolint:errcheck // falls back to killing the child only
			_ = windows.CloseHandle(h)                   //nolint:errcheck // best effort
		}
	}
	return cmd.Wait() //nolint:wrapcheck // the caller adds context
}

func newKillOnCloseJob() windows.Handle {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		_ = windows.CloseHandle(job) //nolint:errcheck // nothing to recover
		return 0
	}
	return job
}

func closeJob(job windows.Handle) {
	if job != 0 {
		_ = windows.CloseHandle(job) //nolint:errcheck // best effort
	}
}

// killTree terminates the job of a started cmd, or the process when it has none.
func killTree(cmd *exec.Cmd) {
	if v, ok := jobs.Load(cmd); ok {
		_ = windows.TerminateJobObject(v.(windows.Handle), 1) //nolint:errcheck // nothing left to kill is fine
		return
	}
	if cmd.Process != nil {
		_ = cmd.Process.Kill() //nolint:errcheck // the process may already have exited
	}
}

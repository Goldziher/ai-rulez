//go:build windows

package npmwatch

import (
	"strconv"
	"unsafe"

	"github.com/samber/oops"
	"golang.org/x/sys/windows"
)

const wrapperLostExitCode = 1

// Start watches the wrapper and attaches the CLI to a job before it can spawn
// children. An inherited marker for a different parent is ignored.
func Start(expectedParent string, parentPID int) error {
	if expectedParent == "" || parentPID <= 0 || expectedParent != strconv.Itoa(parentPID) {
		return nil
	}

	pid := uint32(parentPID) //nolint:gosec // Windows PIDs are DWORD values.
	parent, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return oops.Wrapf(err, "open npm wrapper process")
	}
	if err := verifyParent(parent); err != nil {
		_ = windows.CloseHandle(parent) //nolint:errcheck // the startup check has already failed.
		return err
	}

	job := attachJob()
	go watchParent(parent, job)
	return nil
}

// verifyParent checks that the process had already started when the CLI was
// created and is still running. The opened handle then stays bound to it.
func verifyParent(parent windows.Handle) error {
	var parentStart, cliStart, exitTime, kernelTime, userTime windows.Filetime
	if err := windows.GetProcessTimes(parent, &parentStart, &exitTime, &kernelTime, &userTime); err != nil {
		return oops.Wrapf(err, "read npm wrapper creation time")
	}
	if err := windows.GetProcessTimes(windows.CurrentProcess(), &cliStart, &exitTime, &kernelTime, &userTime); err != nil {
		return oops.Wrapf(err, "read CLI creation time")
	}
	if parentStart.Nanoseconds() > cliStart.Nanoseconds() {
		return oops.Errorf("npm wrapper exited before the CLI started")
	}
	status, err := windows.WaitForSingleObject(parent, 0)
	if err != nil {
		return oops.Wrapf(err, "check npm wrapper process")
	}
	if status != uint32(windows.WAIT_TIMEOUT) {
		return oops.Errorf("npm wrapper exited before the CLI started")
	}
	return nil
}

func attachJob() windows.Handle {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE | windows.JOB_OBJECT_LIMIT_BREAKAWAY_OK
	_, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))) //nolint:gosec // Win32 requires the address of the limits.
	if err != nil || windows.AssignProcessToJobObject(job, windows.CurrentProcess()) != nil {
		_ = windows.CloseHandle(job) //nolint:errcheck // no usable job to keep.
		return 0
	}

	// Keep the handle for the CLI's lifetime. Closing it kills the whole
	// tree even if the wrapper's libuv job kills the CLI before it can react.
	return job
}

func watchParent(parent, job windows.Handle) {
	defer windows.CloseHandle(parent) //nolint:errcheck // the wrapper has already exited.
	status, err := windows.WaitForSingleObject(parent, windows.INFINITE)
	if err != nil || status != windows.WAIT_OBJECT_0 {
		return
	}
	if job != 0 && windows.TerminateJobObject(job, wrapperLostExitCode) == nil {
		return
	}

	// A restrictive enclosing job can prevent assignment to our own job.
	if err := windows.TerminateProcess(windows.CurrentProcess(), wrapperLostExitCode); err != nil {
		windows.ExitProcess(wrapperLostExitCode)
	}
}

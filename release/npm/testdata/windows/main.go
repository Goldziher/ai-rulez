//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	"golang.org/x/sys/windows"

	"github.com/Goldziher/ai-rulez/v5/internal/npmwatch"
)

func main() {
	if err := npmwatch.Start(os.Getenv(npmwatch.ParentEnvironment), os.Getppid()); err != nil {
		panic(err)
	}
	switch os.Args[1] {
	case "owner":
		fmt.Fprintln(os.Stdout, npmwatch.OwnsJob())
	case "status":
		fmt.Fprintln(os.Stdout, os.Args[2])
		fmt.Fprintln(os.Stderr, os.Args[3])
		os.Exit(23)
	case "wait", "breakaway":
		exe, err := os.Executable()
		if err != nil {
			panic(err)
		}
		child := exec.Command(exe, "child")
		if os.Args[1] == "breakaway" {
			if !npmwatch.OwnsJob() {
				panic("npm job is required for the breakaway fixture")
			}
			child.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_BREAKAWAY_FROM_JOB}
		}
		if err := child.Start(); err != nil {
			panic(err)
		}
		fmt.Fprintln(os.Stdout, os.Getpid(), child.Process.Pid)
		if os.Args[1] == "breakaway" {
			return
		}
		if err := child.Wait(); err != nil {
			panic(err)
		}
	case "child":
		time.Sleep(15 * time.Second)
	}
}

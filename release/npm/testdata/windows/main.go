//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"time"
)

func main() {
	switch os.Args[1] {
	case "status":
		fmt.Fprintln(os.Stdout, os.Args[2])
		fmt.Fprintln(os.Stderr, os.Args[3])
		os.Exit(23)
	case "wait":
		exe, err := os.Executable()
		if err != nil {
			panic(err)
		}
		child := exec.Command(exe, "child")
		if err := child.Start(); err != nil {
			panic(err)
		}
		fmt.Fprintln(os.Stdout, os.Getpid(), child.Process.Pid)
		if err := child.Wait(); err != nil {
			panic(err)
		}
	case "child":
		time.Sleep(15 * time.Second)
	}
}

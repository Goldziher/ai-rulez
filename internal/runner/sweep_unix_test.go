//go:build !windows

package runner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"
)

// A short-lived run must learn the root's start time at attach: the sweep that
// follows bounds its search by it, and without it every process of the machine
// is a candidate.
func TestShortLivedAttachRecordsTheRootStart(t *testing.T) {
	// Arrange
	cmd := exec.CommandContext(context.Background(), "/bin/sleep", "30")
	tree := configure(cmd)
	tree.shortLived()
	defer tree.close()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Wait() }() //nolint:errcheck // test
	// Act
	tree.attach(cmd)
	got := tree.rootStart
	tree.kill(cmd)
	// Assert
	if got <= 0 {
		t.Fatalf("rootStart = %d after attach, want the root's start time", got)
	}
}

// The sweep must probe only processes that started no earlier than the root:
// a descendant cannot be older than its ancestor.
func TestAdoptCarriersProbesOnlyProcessesStartedAfterTheRoot(t *testing.T) {
	// Arrange
	tree := &procTree{tracked: map[int]int64{}, checked: map[int]int64{}, token: "tok", rootStart: 1000}
	var probed []int
	tree.probe = func(pid int) bool { probed = append(probed, pid); return pid == 30 }
	table := []procEntry{
		{pid: 10, start: 999, uid: -1},  // older than the root
		{pid: 20, start: 1000, uid: -1}, // same tick as the root
		{pid: 30, start: 1500, uid: -1}, // newer, carries the token
	}
	// Act
	tree.adoptCarriers(table)
	// Assert
	if len(probed) != 2 || probed[0] != 20 || probed[1] != 30 {
		t.Fatalf("probed %v, want [20 30]", probed)
	}
	if _, ok := tree.tracked[30]; !ok {
		t.Fatal("the carrier was not adopted")
	}
	if _, ok := tree.tracked[10]; ok {
		t.Fatal("a process older than the root was adopted")
	}
}

// A short-lived run still kills a detached grandchild that outlived its parent
// (the double fork whose middle process exits at once), and leaves an
// unrelated process that started during the run alone.
func TestShortLivedSweepKillsDetachedGrandchildAndSparesStrangers(t *testing.T) {
	// Arrange
	bystander := exec.Command("/bin/sleep", "60")
	if err := bystander.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bystander.Process.Kill(); _ = bystander.Wait() }) //nolint:errcheck // test
	pidfile := filepath.Join(t.TempDir(), "pid")
	spec := helperSpec(t, "fastdaemon", pidfile, "1500ms")
	spec.ShortLived = true
	spec.Timeout = 10 * time.Second
	// Act
	res := Run(context.Background(), spec)
	// Assert
	if res.Status != StatusOK {
		t.Fatalf("status = %s (%v)", res.Status, res.Err)
	}
	data, err := os.ReadFile(pidfile)
	if err != nil {
		t.Fatalf("the helper never reported its pid: %v", err)
	}
	pid, _ := strconv.Atoi(string(data)) //nolint:errcheck // checked below
	if pid <= 1 {
		t.Fatalf("bad helper pid %q", data)
	}
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) }) //nolint:errcheck // leave nothing behind
	for deadline := time.Now().Add(3 * time.Second); !gone(pid); time.Sleep(20 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("detached grandchild %d outlived the short-lived run", pid)
		}
	}
	if err := syscall.Kill(bystander.Process.Pid, 0); err != nil {
		t.Fatalf("an unrelated process was killed: %v", err)
	}
}

//go:build !windows

package runner

import (
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

func isWindows() bool { return false }

// procEntry is one row of the process table: enough to tell a process's
// parent, group and session, and (with start) to tell it from a later process
// that reuses its pid. sid is the session id where the platform reports one,
// else 0.
type procEntry struct {
	pid, ppid, pgid, sid int
	start                int64
}

// Polling bounds for the descendant tracker: the first look comes quickly, so a
// helper that detaches early is still seen while its parent is alive, then the
// interval backs off.
const (
	pollFirst = 2 * time.Millisecond
	pollMax   = 200 * time.Millisecond
	// stopRounds bounds the freeze loop in kill.
	stopRounds = 8
)

// procTree owns the processes a command started: its process group, and every
// descendant seen while the command ran. A helper that leaves the group with
// setsid() or setpgid() is still a descendant, so it is tracked and killed
// with the rest; one that detaches and loses its parent between two looks at
// the process table can escape (a double fork faster than the poll).
type procTree struct {
	mu        sync.Mutex
	root      int
	rootStart int64
	// rootReused is set once the root's pid belongs to another process.
	rootReused bool
	tracked    map[int]int64 // pid -> start time
	stop       chan struct{}
	done       chan struct{}
}

// configure starts the child in a new session (which is also a new process
// group) and makes cancellation kill the whole tree, so a scanner's helpers die
// with it. The new session leaves the child without a controlling terminal: a
// command run from an interactive shell cannot open /dev/tty and push input
// into the user's terminal (TIOCSTI).
func configure(cmd *exec.Cmd) *procTree {
	t := &procTree{tracked: map[int]int64{}}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Cancel = func() error {
		t.kill(cmd)
		return nil
	}
	return t
}

// attach starts watching the process table for the started child's descendants.
func (t *procTree) attach(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	table, err := processTable()
	t.mu.Lock()
	t.root = cmd.Process.Pid
	for _, p := range table {
		if p.pid == t.root {
			t.rootStart = p.start
		}
	}
	t.mu.Unlock()
	if err != nil {
		return // no process table: the group kill is all there is
	}
	t.stop, t.done = make(chan struct{}), make(chan struct{})
	go t.watch()
}

func (t *procTree) watch() {
	defer close(t.done)
	d := pollFirst
	timer := time.NewTimer(d)
	defer timer.Stop()
	for {
		select {
		case <-t.stop:
			return
		case <-timer.C:
		}
		if table, err := processTable(); err == nil {
			t.mu.Lock()
			t.collect(table)
			t.mu.Unlock()
		}
		d = min(d*2, pollMax)
		timer.Reset(d)
	}
}

// collect adds to tracked every process of table that belongs to the tree: in
// the root's group or session, or a descendant of a member. It returns the
// members alive in table. The caller holds mu.
func (t *procTree) collect(table []procEntry) []procEntry {
	children := map[int][]procEntry{}
	for _, p := range table {
		children[p.ppid] = append(children[p.ppid], p)
	}
	rootOurs := t.identifyRoot(table)
	self := os.Getpid()
	member := map[int]procEntry{}
	var queue []procEntry
	add := func(p procEntry) {
		if _, ok := member[p.pid]; !ok && p.pid > 1 && p.pid != self {
			member[p.pid] = p
			queue = append(queue, p)
		}
	}
	for _, p := range table {
		if t.isSeed(p, rootOurs) {
			add(p)
		}
	}
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		for _, c := range children[p.pid] {
			add(c)
		}
	}
	out := make([]procEntry, 0, len(member))
	for pid, p := range member {
		t.tracked[pid] = p.start
		out = append(out, p)
	}
	return out
}

// identifyRoot records the root's start time the first time table shows it and
// reports whether the root's pid is still ours. A reaped root whose pid was
// reused is not: the kernel does not hand out a pid still in use as a group or
// session id, so nothing of ours is left in that group and the group test would
// match strangers.
func (t *procTree) identifyRoot(table []procEntry) bool {
	for _, p := range table {
		if p.pid != t.root {
			continue
		}
		if t.rootStart == 0 {
			t.rootStart = p.start // canceled before attach: the root is not reaped yet
		}
		if p.start != t.rootStart {
			t.rootReused = true
			return false
		}
	}
	return true
}

// isSeed reports whether p is a tracked process, or (while the root's pid is
// ours) the root or a process in its group or session.
func (t *procTree) isSeed(p procEntry, rootOurs bool) bool {
	if start, ok := t.tracked[p.pid]; ok && start == p.start {
		return true
	}
	return rootOurs && t.root > 1 && (p.pid == t.root || p.pgid == t.root || p.sid == t.root)
}

// kill ends the whole tree: the process group, and every tracked descendant
// still alive. Members are stopped first so none can fork a child that would
// be reparented away before the kill; the table is read again until no new
// member appears. It is a no-op once nothing is left.
func (t *procTree) kill(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.root == 0 {
		t.root = cmd.Process.Pid // canceled before attach: the group kill below still applies
	}
	stopped := map[int]bool{}
	for range stopRounds {
		table, err := processTable()
		if err != nil {
			break
		}
		fresh := false
		for _, p := range t.collect(table) {
			if !stopped[p.pid] {
				stopped[p.pid], fresh = true, true
				_ = syscall.Kill(p.pid, syscall.SIGSTOP) //nolint:errcheck // gone already is fine
			}
		}
		if !fresh {
			break
		}
	}
	if !t.rootReused {
		_ = syscall.Kill(-t.root, syscall.SIGKILL) //nolint:errcheck // ESRCH when nothing is left
	}
	for pid := range stopped {
		_ = syscall.Kill(pid, syscall.SIGKILL) //nolint:errcheck // gone already is fine
	}
}

// close stops the watcher.
func (t *procTree) close() {
	if t.stop != nil {
		close(t.stop)
		<-t.done
		t.stop = nil
	}
}

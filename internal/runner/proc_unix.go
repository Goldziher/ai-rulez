//go:build !windows

package runner

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

func isWindows() bool { return false }

// procEntry is one row of the process table: enough to tell a process's
// parent, group and session, and (with start) to tell it from a later process
// that reuses its pid. sid is the session id where the platform reports one,
// else 0. uid is the real user id where the table reports it, else -1.
type procEntry struct {
	pid, ppid, pgid, sid int
	uid                  int
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
// with the rest. One that detaches and loses its parent between two looks at
// the process table (a double fork faster than the poll) is found by the run
// token every child inherits in its environment (RunTokenEnv), or by the
// write end of the marker pipe every child inherits as an extra descriptor.
// macOS withholds the environment of hardened and platform binaries (/bin/sh,
// perl), so there the descriptor is what finds them. Only a helper that drops
// both, the variable and the descriptor, before it detaches escapes.
type procTree struct {
	mu        sync.Mutex
	root      int
	rootStart int64
	// rootReused is set once the root's pid belongs to another process.
	rootReused bool
	tracked    map[int]int64 // pid -> start time
	// token is this run's value of RunTokenEnv, "" when none could be made.
	token string
	// checked caches the processes already searched for the token (pid -> start).
	checked map[int]int64
	// markR and markW are the marker pipe; markW is the child's extra
	// descriptor and is closed here once the child has started. markID
	// identifies the pipe (0 when there is none).
	markR, markW *os.File
	markID       uint64
	stop         chan struct{}
	done         chan struct{}
	// short marks a trusted, short-lived command: no watcher polls the process
	// table while it runs, and the sweep after it ends reads the table once
	// when nothing is left. The run token and marker descriptor still find a
	// helper that detached.
	short bool
}

// shortLived marks the tree's command as trusted and short-lived (see Spec.ShortLived).
func (t *procTree) shortLived() { t.short = true }

// configure starts the child in a new session (which is also a new process
// group) and makes cancellation kill the whole tree, so a scanner's helpers die
// with it. The new session leaves the child without a controlling terminal: a
// command run from an interactive shell cannot open /dev/tty and push input
// into the user's terminal (TIOCSTI). The child's environment carries a fresh
// run token and an extra descriptor on the marker pipe, so a helper that leaves
// the tree is still recognized as the run's.
func configure(cmd *exec.Cmd) *procTree {
	t := &procTree{tracked: map[int]int64{}, checked: map[int]int64{}, token: newRunToken()}
	if t.token != "" {
		cmd.Env = withRunToken(cmd.Env, t.token)
	}
	if r, w, err := os.Pipe(); err == nil {
		if id, err := markerID(r); err == nil && id != 0 {
			t.markR, t.markW, t.markID = r, w, id
			cmd.ExtraFiles = append(cmd.ExtraFiles, w)
		} else {
			_ = r.Close() //nolint:errcheck // unused
			_ = w.Close() //nolint:errcheck // unused
		}
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Cancel = func() error {
		t.kill(cmd)
		return nil
	}
	return t
}

// attach starts watching the process table for the started child's descendants.
func (t *procTree) attach(cmd *exec.Cmd) {
	if t.markW != nil {
		_ = t.markW.Close() //nolint:errcheck // the child has its copy
		t.markW = nil
	}
	if cmd.Process == nil {
		return
	}
	if t.short {
		t.mu.Lock()
		t.root = cmd.Process.Pid
		t.mu.Unlock()
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

// kill ends the whole tree. One read of the process table records the
// descendants that left the group, then the group is killed at once (as before
// the tree was tracked, so the run's pipes close without waiting for the
// sweep); then every other tracked process is stopped, so none can fork a child
// that would be reparented away, the table is read again until no new member
// appears, and the stopped ones are killed. It is a no-op once nothing is left.
func (t *procTree) kill(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.root == 0 {
		t.root = cmd.Process.Pid // canceled before attach: the group kill below still applies
	}
	table, tableErr := processTable()
	var members []procEntry
	if tableErr == nil {
		if t.short {
			t.adoptCarriers(table)
		}
		members = t.collect(table)
	}
	if !t.rootReused {
		_ = syscall.Kill(-t.root, syscall.SIGKILL) //nolint:errcheck // ESRCH when nothing is left
	}
	if t.short && tableErr == nil && len(members) == 0 {
		return // nothing of the run is left: one read of the table was enough
	}
	stopped := map[int]bool{}
	for range stopRounds {
		table, err := processTable()
		if err != nil {
			break
		}
		t.adoptCarriers(table)
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
	for pid := range stopped {
		_ = syscall.Kill(pid, syscall.SIGKILL) //nolint:errcheck // gone already is fine
	}
}

// adoptCarriers adds to tracked every process of table that carries this run's
// token in its environment or holds the marker pipe: a helper that detached and
// was reparented before the watcher saw it. Only the current user's processes
// started no earlier than the root are searched, each once. The caller holds mu.
func (t *procTree) adoptCarriers(table []procEntry) {
	if t.token == "" && t.markID == 0 {
		return
	}
	self, uid := os.Getpid(), os.Getuid()
	for _, p := range table {
		if p.pid <= 1 || p.pid == self || (t.rootStart != 0 && p.start < t.rootStart) {
			continue
		}
		if start, ok := t.tracked[p.pid]; ok && start == p.start {
			continue
		}
		if start, ok := t.checked[p.pid]; ok && start == p.start {
			continue
		}
		t.checked[p.pid] = p.start
		if p.uid >= 0 && p.uid != uid {
			continue
		}
		if t.carries(p.pid) {
			t.tracked[p.pid] = p.start
		}
	}
}

// carries reports whether pid has this run's token or marker descriptor.
func (t *procTree) carries(pid int) bool {
	if t.token != "" {
		if env, err := processEnv(pid); err == nil && hasRunToken(env, t.token) {
			return true
		}
	}
	return t.markID != 0 && holdsMarker(pid, t.markID)
}

// newRunToken returns a random token, or "" if the system has no randomness.
func newRunToken() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ""
	}
	return hex.EncodeToString(b[:])
}

// withRunToken returns env (nil meaning the parent's environment) with token
// appended to RunTokenEnv.
func withRunToken(env []string, token string) []string {
	if env == nil {
		env = os.Environ()
	}
	out := make([]string, 0, len(env)+1)
	value := token
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, RunTokenEnv+"="); ok {
			if v != "" {
				value = v + ":" + token
			}
			continue
		}
		out = append(out, kv)
	}
	return append(out, RunTokenEnv+"="+value)
}

// hasRunToken reports whether env (KEY=VALUE entries) tags its process with token.
func hasRunToken(env []string, token string) bool {
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, RunTokenEnv+"="); ok {
			for _, t := range strings.Split(v, ":") {
				if t == token {
					return true
				}
			}
		}
	}
	return false
}

// close stops the watcher.
func (t *procTree) close() {
	if t.stop != nil {
		close(t.stop)
		<-t.done
		t.stop = nil
	}
	for _, f := range []*os.File{t.markW, t.markR} {
		if f != nil {
			_ = f.Close() //nolint:errcheck // nothing to recover
		}
	}
	t.markW, t.markR = nil, nil
}

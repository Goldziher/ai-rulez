package runner

import (
	"bytes"
	"os"
	"strconv"
)

// processTable lists every process of the system (see procEntry) from /proc.
func processTable() ([]procEntry, error) {
	ents, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err //nolint:wrapcheck // the caller falls back to the group kill
	}
	out := make([]procEntry, 0, len(ents))
	for _, e := range ents {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		data, err := os.ReadFile("/proc/" + e.Name() + "/stat")
		if err != nil {
			continue // exited since the directory was read
		}
		if p, ok := parseStat(pid, data); ok {
			out = append(out, p)
		}
	}
	return out, nil
}

// parseStat reads ppid, pgrp, session and starttime from a /proc/<pid>/stat
// line. The command name (field 2) may hold spaces and parentheses, so the
// fields are counted from the last ')'.
func parseStat(pid int, data []byte) (procEntry, bool) {
	i := bytes.LastIndexByte(data, ')')
	if i < 0 {
		return procEntry{}, false
	}
	f := bytes.Fields(data[i+1:])
	// f[0] is field 3 (state); starttime is field 22.
	if len(f) < 20 {
		return procEntry{}, false
	}
	num := func(b []byte) int64 {
		n, err := strconv.ParseInt(string(b), 10, 64)
		if err != nil {
			return -1
		}
		return n
	}
	p := procEntry{pid: pid, ppid: int(num(f[1])), pgid: int(num(f[2])), sid: int(num(f[3])), uid: -1, start: num(f[19])}
	return p, p.ppid >= 0 && p.start >= 0
}

// processStart returns the start time of pid (same clock as procEntry.start)
// from its own stat file, not a read of the whole table.
func processStart(pid int) (int64, bool) {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0, false
	}
	p, ok := parseStat(pid, data)
	return p.start, ok
}

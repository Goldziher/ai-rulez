//go:build !windows

package runner

import (
	"context"
	"os/exec"
	"testing"
)

// BenchmarkSpawnCleanup times one spawn of a trivial command including the
// sweep that follows it, for the trusted short-lived path (git) and the full one.
func BenchmarkSpawnCleanup(b *testing.B) {
	for _, short := range []bool{true, false} {
		name := "full"
		if short {
			name = "short"
		}
		b.Run(name, func(b *testing.B) {
			spec := Spec{Argv: []string{"/bin/sh", "-c", "exit 0"}, Env: []string{"PATH=/usr/bin:/bin"}, ShortLived: short}
			b.ReportAllocs()
			for range b.N {
				if res := Run(context.Background(), spec); res.Status != StatusOK {
					b.Fatalf("status %s: %v", res.Status, res.Err)
				}
			}
		})
	}
}

// BenchmarkAdoptCarriers times the sweep alone over the real process table of
// the machine: "unbounded" is what a run whose root start is unknown pays (every
// process of the user is searched), "bounded" what a run pays that knows it.
func BenchmarkAdoptCarriers(b *testing.B) {
	cmd := exec.CommandContext(context.Background(), "/bin/sleep", "60")
	if err := cmd.Start(); err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }) //nolint:errcheck // test
	start, ok := processStart(cmd.Process.Pid)
	if !ok {
		b.Skip("no process start time here")
	}
	table, err := processTable()
	if err != nil {
		b.Skipf("no process table: %v", err)
	}
	b.Logf("%d processes in the table", len(table))
	for _, tt := range []struct {
		name  string
		start int64
	}{{"unbounded", 0}, {"bounded", start}} {
		b.Run(tt.name, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				tree := &procTree{tracked: map[int]int64{}, checked: map[int]int64{}, token: newRunToken(), rootStart: tt.start}
				tree.probe = tree.carries
				tree.adoptCarriers(table)
			}
		})
	}
}

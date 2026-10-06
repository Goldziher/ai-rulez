package sandbox

import (
	"net"
	"testing"
	"time"
)

// netProbe connects to addr: "allowed" when it works, "denied" when the sandbox
// refuses it. It dials rather than listens: a bubblewrap network namespace has
// its own loopback, so listening there succeeds, while reaching a listener of
// the parent's network does not.
func netProbe(addr string) string {
	c, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return "denied"
	}
	_ = c.Close() //nolint:errcheck // probe
	return "allowed"
}

// listenHere starts a listener in the test process (outside any sandbox) and
// returns its address.
func listenHere(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("cannot listen on loopback: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() }) //nolint:errcheck // test
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			_ = c.Close() //nolint:errcheck // test
		}
	}()
	return l.Addr().String()
}

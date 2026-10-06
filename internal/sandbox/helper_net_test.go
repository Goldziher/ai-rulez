package sandbox

import "net"

// netProbe opens a loopback listener: "allowed" when it works, "denied" when the
// sandbox refuses the socket.
func netProbe() string {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "denied"
	}
	_ = l.Close() //nolint:errcheck // probe
	return "allowed"
}

package commands

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"strings"
)

// daemonIdentity names the shared daemon a command line uses: the ai-rulez
// version and every argument after the program name. Two MCP entries that differ
// in any flag (a profile, a root, a usage sink) therefore never share a daemon,
// and the spawned child, which is given the same arguments plus the hidden
// --daemon-server marker, agrees with its proxy: the marker is left out. It is
// the identity the socket path is keyed by, together with the repository
// toplevel.
func daemonIdentity() string {
	h := sha256.New()
	io.WriteString(h, Version) //nolint:errcheck,gosec // hash writes never fail
	io.WriteString(h, "\x00")  //nolint:errcheck,gosec // hash writes never fail
	for _, arg := range os.Args[1:] {
		if isDaemonServerArg(arg) {
			continue
		}
		io.WriteString(h, arg)    //nolint:errcheck,gosec // hash writes never fail
		io.WriteString(h, "\x00") //nolint:errcheck,gosec // hash writes never fail
	}
	return hex.EncodeToString(h.Sum(nil))
}

// isDaemonServerArg reports whether arg is the hidden daemon-server marker, in
// any spelling cobra accepts.
func isDaemonServerArg(arg string) bool {
	return arg == "--"+flagServeDaemonServer || arg == "-"+flagServeDaemonServer ||
		strings.HasPrefix(arg, "--"+flagServeDaemonServer+"=")
}

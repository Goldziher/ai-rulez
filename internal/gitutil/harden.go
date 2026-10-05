package gitutil

// HardenedProtocols is the GIT_ALLOW_PROTOCOL value of a hardened fetch: the
// transports that run no program. The ext:: transport (which runs a command) and
// the plain git:// and http:// ones are not among them.
const HardenedProtocols = "file:https:ssh"

// BaselineConfig returns the `-c` arguments every git fetch gets, hardened or
// not: the ext:: transport runs an arbitrary command named in the URL and is
// never allowed.
func BaselineConfig() []string { return []string{"-c", "protocol.ext.allow=never"} }

// BaselineProtocols is the GIT_ALLOW_PROTOCOL value of a fetch that is not fully
// hardened (an include): everything a repository URL can reasonably use except
// ext:: and other program-running transports.
const BaselineProtocols = "file:git:http:https:ssh"

// HardenedConfig returns the `-c key=value` arguments for a git invocation that
// fetches content from a source the user does not control (an OKF bundle): no
// hooks, no helpers that could prompt or run programs, no submodules, and only
// the https, ssh and file transports.
func HardenedConfig() []string {
	return []string{
		"-c", "core.hooksPath=/dev/null", "-c", "protocol.allow=never", "-c", "protocol.ext.allow=never",
		"-c", "protocol.https.allow=always", "-c", "protocol.ssh.allow=always", "-c", "protocol.file.allow=always",
		"-c", "credential.helper=", "-c", "core.fsmonitor=false", "-c", "submodule.recurse=false",
	}
}

// HardenedEnv adds to environ (os.Environ when nil, with the repository-selecting
// variables dropped) the settings that keep git from prompting, reading system
// configuration or downloading LFS objects.
func HardenedEnv(environ []string) []string {
	return append(Env(environ), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=true", "GIT_CONFIG_NOSYSTEM=1", "GIT_LFS_SKIP_SMUDGE=1", "GIT_ALLOW_PROTOCOL="+HardenedProtocols)
}

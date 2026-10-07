package settings

// Tool names of the Claude-style rule grammar that several translators match or
// emit.
const (
	toolBash      = "Bash"
	toolRead      = "Read"
	toolEdit      = "Edit"
	toolWrite     = "Write"
	toolWebFetch  = "WebFetch"
	toolWebSearch = "WebSearch"
	toolAgent     = "Agent"
)

// Keys of the harness documents that more than one translator writes.
const (
	keyHooks      = "hooks"
	keyPermission = "permission"
	keyTools      = "tools"
	keyTerminal   = "terminal"
)

// Hook events that only some vendors spell.
const (
	eventPermissionDenied   = "PermissionDenied"
	eventStopFailure        = "StopFailure"
	eventInstructionsLoaded = "InstructionsLoaded"
)

// msgOnlyWebFetchDomain is the reason a network rule other than a WebFetch
// domain is dropped by the harnesses that scope the network by host.
const msgOnlyWebFetchDomain = "only WebFetch(domain:host) rules carry over"

// Native tool names of the harnesses whose rule grammar names tools instead of
// using the Claude-style ones.
const (
	nativeReadFile  = "read_file"
	nativeWriteFile = "write_file"
	nativeWebFetch  = "web_fetch"
)

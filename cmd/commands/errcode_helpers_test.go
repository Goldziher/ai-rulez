package commands

// codeOf is the process exit code a returned error ends a command with; 0 for nil.
func codeOf(err error) int {
	if err == nil {
		return exitOK
	}
	return exitCodeFor(err)
}

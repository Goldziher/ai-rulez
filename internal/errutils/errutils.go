package errutils

// LogIfErr - ignores the error (used to silence linter warnings for intentionally ignored errors)
func LogIfErr(err error) {
	// Intentionally empty - used to silence linter warnings
	// for errors that are safe to ignore in specific contexts
	_ = err
}

// Must - panics if the error is not nil (used for setup errors that should never happen)
func Must(err error) {
	if err != nil {
		panic(err)
	}
}

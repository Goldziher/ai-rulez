package config

import "time"

// localLockTimeout bounds how long an overlay edit waits for another editor, and
// localLockPoll is the retry interval. Variables so tests can shorten them.
var (
	localLockTimeout = 10 * time.Second
	localLockPoll    = 20 * time.Millisecond
)

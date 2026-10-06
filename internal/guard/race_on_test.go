//go:build race

package guard

// raceEnabled turns timing assertions off: the race detector slows the guard 20-80x.
const raceEnabled = true

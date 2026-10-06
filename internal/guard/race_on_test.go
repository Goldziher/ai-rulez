//go:build race

package guard

// raceEnabled scales timing assertions: the race detector slows the guard about tenfold.
const raceEnabled = true

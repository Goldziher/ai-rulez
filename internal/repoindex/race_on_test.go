//go:build race

package repoindex

// raceEnabled turns the timing guard off: the race detector slows the build 5-20x.
const raceEnabled = true

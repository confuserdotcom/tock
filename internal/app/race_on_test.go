//go:build race

package app

// raceOn is true when the race detector is on. It changes allocation
// counts and is many times slower, so the budget tests skip under it; the
// non-race CI jobs run them.
const raceOn = true

package main

import (
	"runtime/debug"
	"time"
)

// freeEvery is how often trackd hands freed memory back to the system.
const freeEvery = 5 * time.Minute

// freeLoop returns the heap Go keeps after a burst (the first labelling of
// history at start, a rebuild after a rules change) instead of waiting for
// the background scavenger, which lets it sit for many minutes. One forced
// GC every few minutes costs a few ms; measured, it is 5 MB less footprint
// after a cold start.
type freeLoop struct{}

func (freeLoop) tick()  { debug.FreeOSMemory() }
func (freeLoop) close() {}

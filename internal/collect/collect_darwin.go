//go:build darwin && cgo

package collect

/*
#cgo CFLAGS: -fobjc-arc
#cgo LDFLAGS: -framework Cocoa -framework ApplicationServices -framework CoreServices
#include <stdlib.h>
#include "shim.h"
*/
import "C"

import (
	"errors"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"tock/internal/span"
)

// Pin main.main to the first OS thread: every Cocoa call must run there.
func init() { runtime.LockOSThread() }

// The exported callbacks are invoked from C with no receiver, so the source is
// a process-wide singleton.
var (
	active   *darwinSource
	activeMu sync.Mutex
)

type darwinSource struct {
	opts    Options
	ch      chan span.Input
	dropped atomic.Uint64
	stop    sync.Once
}

// NewDarwin returns the macOS source. Only one may exist per process.
func NewDarwin(opts Options) (Source, error) {
	activeMu.Lock()
	defer activeMu.Unlock()
	if active != nil {
		return nil, errors.New("collect: darwin source already created")
	}
	if opts.PollInterval <= 0 {
		opts.PollInterval = DefaultPollInterval
	}
	active = &darwinSource{opts: opts, ch: make(chan span.Input, ChanBuffer)}
	return active, nil
}

func (s *darwinSource) Inputs() <-chan span.Input { return s.ch }
func (s *darwinSource) Dropped() uint64           { return s.dropped.Load() }

// Run blocks on the main thread's run loop until Stop is called.
func (s *darwinSource) Run() error {
	defer close(s.ch)
	C.tock_start_observers()
	C.tock_start_poll_timer(C.double(s.opts.PollInterval.Seconds()))
	C.tock_run_loop()
	return nil
}

// Stop ends the run loop. Safe from any goroutine.
func (s *darwinSource) Stop() {
	s.stop.Do(func() { C.tock_stop_run_loop() })
}

// trySend never blocks the run loop thread; a full channel drops the sample.
func (s *darwinSource) trySend(in span.Input) {
	select {
	case s.ch <- in:
	default:
		s.dropped.Add(1)
	}
}

func current() *darwinSource {
	activeMu.Lock()
	defer activeMu.Unlock()
	return active
}

//export goActivation
func goActivation(bundle, app *C.char, ts C.longlong) {
	s := current()
	if s == nil {
		return
	}
	// C strings die when the callback returns: copy now.
	smp := &span.Sample{
		TS:         int64(ts),
		BundleID:   strings.ToValidUTF8(C.GoString(bundle), "�"),
		App:        strings.ToValidUTF8(C.GoString(app), "�"),
		Activation: true,
	}
	s.trySend(span.Input{Sample: smp})
}

//export goPollTick
func goPollTick() {
	s := current()
	if s == nil {
		return
	}
	var p C.tock_poll_t
	C.tock_poll(&p)
	smp := &span.Sample{
		TS:       time.Now().Round(0).UnixMilli(),
		BundleID: strings.ToValidUTF8(C.GoString(&p.bundle[0]), "�"),
		App:      strings.ToValidUTF8(C.GoString(&p.app[0]), "�"),
	}
	s.trySend(span.Input{Sample: smp})
}

//export goEvent
func goEvent(kind C.int, ts C.longlong) {
	s := current()
	if s == nil {
		return
	}
	// Events are never dropped, so this send blocks if the channel is full.
	s.ch <- span.Input{Event: &span.Event{TS: int64(ts), Kind: span.EventKind(kind)}}
}

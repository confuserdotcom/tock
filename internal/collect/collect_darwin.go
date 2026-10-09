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

	// ax and axStale are touched only on the main thread (poll, activation
	// and Run all execute there), so they need no lock.
	ax      axTracker
	axStale bool
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
	prompt := C.int(0)
	if s.opts.PromptAX {
		prompt = 1
	}
	C.tock_ax_init(prompt)
	C.tock_start_observers()
	C.tock_start_poll_timer(C.double(s.opts.PollInterval.Seconds()))
	C.tock_run_loop()
	if s.axStale {
		return ErrAXStale
	}
	return nil
}

// feedAX records the poll's trust state and AX error. The trust transition is
// reported by the daemon through the Sample's AXTrusted field; the collector
// itself does not log. A stuck grant stops the run loop so Run can return
// ErrAXStale.
func (s *darwinSource) feedAX(p *C.tock_poll_t) {
	_, _, stale := s.ax.observe(p.ax_trusted != 0, int(p.ax_err))
	if stale && !s.axStale {
		s.axStale = true
		C.tock_stop_run_loop()
	}
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
	nBundle := strings.ToValidUTF8(C.GoString(bundle), "�")
	nApp := strings.ToValidUTF8(C.GoString(app), "�")

	// Called about 150 ms after the notification, so the poll sees the settled
	// window. The sample is stamped with the notification receipt time.
	var p C.tock_poll_t
	C.tock_poll(&p)
	s.feedAX(&p)
	smp := sampleFromPoll(&p)
	polledBundle := smp.BundleID
	smp.TS = int64(ts)
	smp.BundleID = nBundle
	smp.App = nApp
	smp.Activation = true
	if polledBundle != nBundle {
		// That title belongs to another app (Pitfall 8).
		smp.Title = nil
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
	s.feedAX(&p)
	smp := sampleFromPoll(&p)
	smp.TS = time.Now().Round(0).UnixMilli()
	s.trySend(span.Input{Sample: smp})
}

// sampleFromPoll converts the C poll struct. Title is set only when the shim
// read one; invalid UTF-8 from other processes is replaced.
func sampleFromPoll(p *C.tock_poll_t) *span.Sample {
	smp := &span.Sample{
		BundleID:  strings.ToValidUTF8(C.GoString(&p.bundle[0]), "�"),
		App:       strings.ToValidUTF8(C.GoString(&p.app[0]), "�"),
		IdleS:     float64(p.idle_s),
		Locked:    p.locked != 0,
		AXTrusted: p.ax_trusted != 0,
	}
	if p.has_title != 0 {
		t := strings.ToValidUTF8(C.GoString(&p.title[0]), "�")
		smp.Title = &t
	}
	return smp
}

//export goEvent
func goEvent(kind C.int, ts C.longlong) {
	s := current()
	if s == nil {
		return
	}
	k, ok := eventKindFromCode(int(kind))
	if !ok {
		return
	}
	// Events are never dropped, so this send blocks if the channel is full.
	s.ch <- span.Input{Event: &span.Event{TS: int64(ts), Kind: k}}
}

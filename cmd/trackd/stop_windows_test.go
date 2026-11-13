//go:build windows

package main

import (
	"strings"
	"sync"
	"testing"
	"time"

	"tock/internal/span"
	"tock/internal/stopsignal"
)

// waitSource runs until Stop, like a real collector.
type waitSource struct {
	ch      chan span.Input
	done    chan struct{}
	started chan struct{}
	once    sync.Once
}

func newWaitSource() *waitSource {
	return &waitSource{ch: make(chan span.Input), done: make(chan struct{}), started: make(chan struct{})}
}
func (s *waitSource) Inputs() <-chan span.Input { return s.ch }
func (s *waitSource) Run() error {
	close(s.started)
	<-s.done
	close(s.ch)
	return nil
}
func (s *waitSource) Stop()           { s.once.Do(func() { close(s.done) }) }
func (s *waitSource) Dropped() uint64 { return 0 }

func TestStopEventStopsTrackdCleanly(t *testing.T) {
	src := newWaitSource()
	got := make(chan daemonRun, 1)
	go func() { got <- runFakeDaemon(t, t.TempDir(), src) }()
	select {
	case <-src.started:
	case <-time.After(10 * time.Second):
		t.Fatal("trackd never ran its source")
	}
	var err error
	for i := 0; i < 50; i++ { // the listener may lag the source by a moment
		if err = stopsignal.Send(stopsignal.Trackd); err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	select {
	case r := <-got:
		if r.rc != exitNoRestart || !strings.Contains(r.log, "stop requested") {
			t.Fatalf("rc %d\n%s", r.rc, r.log)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("trackd did not stop")
	}
}

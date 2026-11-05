package main

import (
	"sync"

	"tock/internal/app"
)

// shared is the one app session trackd's focus, sync and prune loops use:
// one database connection and one copy of the rules instead of one each
// (each costs about 5 MB). Their ticks take turns on it.
type shared struct {
	mu   sync.Mutex
	open func() (*app.App, error)
	a    *app.App
}

// opener is the loops' open: the session, opened on first use. Loops call it
// from a tick, which holds mu.
func (s *shared) opener() (*app.App, error) {
	if s.a == nil {
		a, err := s.open()
		if err != nil {
			return nil, err
		}
		s.a = a
	}
	return s.a, nil
}

// turn wraps l so its ticks take turns on the session. Closing it leaves
// the session open; close closes that.
func (s *shared) turn(l loop) loop { return turnLoop{l: l, s: s} }

// close closes the session, after every loop has stopped.
func (s *shared) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.a != nil {
		s.a.Close()
		s.a = nil
	}
}

type turnLoop struct {
	l loop
	s *shared
}

func (t turnLoop) tick() {
	t.s.mu.Lock()
	defer t.s.mu.Unlock()
	t.l.tick()
}

func (turnLoop) close() {}

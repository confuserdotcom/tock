package tui

import (
	"sync"

	"tock/internal/app"
)

// source serialises access to the app: Bubble Tea runs commands
// concurrently and app.App is not safe for concurrent use.
type source struct {
	mu sync.Mutex
	a  *app.App
}

func (s *source) do(fn func(a *app.App)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(s.a)
}

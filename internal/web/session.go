package web

import (
	"errors"
	"fmt"
	"net/http"

	"tock/internal/app"
	"tock/internal/config"
	"tock/internal/store"
)

// session starts or stops the focus session, as the menu bar and the notch
// do: {"action":"start","minutes":50,"goal":"…"} or {"action":"stop"}.
func (s *Server) session(r *http.Request) (any, error) {
	var b struct {
		Action  string `json:"action"`
		Minutes int    `json:"minutes"`
		Goal    string `json:"goal"`
	}
	if err := decode(r, &b); err != nil {
		return nil, err
	}
	var err error
	switch b.Action {
	case "start":
		if b.Minutes < 1 || b.Minutes > config.MaxSessionMin {
			return nil, badRequest(fmt.Sprintf("a session runs 1 to %d minutes", config.MaxSessionMin))
		}
		_, err = s.a.StartSessionGoal(b.Minutes, b.Goal)
	case "stop":
		_, err = s.a.StopSession()
	default:
		return nil, badRequest(`action is "start" or "stop"`)
	}
	switch {
	case errors.Is(err, store.ErrSessionRunning):
		return nil, httpError{http.StatusConflict, "a session is already running"}
	case errors.Is(err, store.ErrNoSession):
		return nil, httpError{http.StatusConflict, "no session is running"}
	case errors.Is(err, app.ErrFocusOff):
		return nil, httpError{http.StatusConflict, err.Error()}
	case err != nil:
		return nil, err
	}
	return s.status(r)
}

package tui

import (
	"context"
	"encoding/json"
	"time"

	tea "charm.land/bubbletea/v2"

	"tock/internal/app"
	"tock/internal/suggest"
)

// suggestEvery is the least time between two background runs. Most runs
// stop at the hourly claim; this only spares the inbox computation.
const suggestEvery = time.Minute

// suggestTimeout bounds one Claude call.
const suggestTimeout = 30 * time.Second

// suggestedMsg ends a background suggestion run.
type suggestedMsg struct {
	n    int
	sent bool
	err  error
}

// WithSuggester enables background suggestions through c.
func (m Model) WithSuggester(c suggest.Client) Model {
	m.suggester = c
	return m
}

// maybeSuggest starts one background run when a client is set, none is in
// flight and the last one started over suggestEvery ago. The network call
// runs outside the app lock, so refreshes continue during it.
func (m *Model) maybeSuggest() tea.Cmd {
	if m.suggester == nil || m.suggesting || m.now().Before(m.suggestAt) {
		return nil
	}
	m.suggesting, m.suggestAt = true, m.now().Add(suggestEvery)
	src, client := m.src, m.suggester
	return func() tea.Msg {
		var p suggest.Prepared
		var err error
		src.do(func(a *app.App) { p, err = a.SuggestPrepare(false) })
		if err != nil || p.Status != suggest.Ready {
			return suggestedMsg{err: err}
		}
		ctx, cancel := context.WithTimeout(context.Background(), suggestTimeout)
		defer cancel()
		var raw json.RawMessage
		if raw, err = client.Suggest(ctx, p.Model, p.Batch.Payload); err != nil {
			return suggestedMsg{err: err}
		}
		var n int
		src.do(func(a *app.App) { n, err = a.SuggestSave(p.Batch, p.Model, raw) })
		return suggestedMsg{n: n, sent: true, err: err}
	}
}

func (m Model) suggested(msg suggestedMsg) (tea.Model, tea.Cmd) {
	m.suggesting = false
	switch {
	case msg.err != nil:
		m.setFlash("suggestions failed: "+msg.err.Error(), true)
	case msg.sent:
		if msg.n > 0 {
			m.setFlash(plural(msg.n, "new suggestion")+" in the inbox", false)
		}
		return m, m.reload()
	}
	return m, nil
}

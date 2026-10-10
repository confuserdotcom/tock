package app

import (
	"encoding/json"

	"tock/internal/ask"
)

// AskPayload is the exact message tock ask sends for question.
func (a *App) AskPayload(question string) ([]byte, error) {
	return ask.Payload(question, a.now(), a.rs)
}

// AskAnswer reads Claude's query and runs it on this machine's data.
func (a *App) AskAnswer(raw json.RawMessage) (ask.Query, ask.Result, error) {
	q, err := ask.Parse(raw, a.rs)
	if err != nil {
		return q, ask.Result{}, err
	}
	from, to := q.Bounds(a.now())
	now := a.nowMS()
	vs, labels, err := a.labelled(from.UnixMilli(), min(to.UnixMilli(), now), now)
	if err != nil {
		return q, ask.Result{}, err
	}
	return q, ask.Run(q, from, to, vs, labels), nil
}

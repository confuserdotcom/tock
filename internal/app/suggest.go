package app

import (
	"encoding/json"
	"strconv"

	"tock/internal/classify"
	"tock/internal/store"
	"tock/internal/suggest"
)

// SuggestBatch returns the batch that would be sent now: eligible, uncached
// inbox keys, filtered by [llm]. It claims nothing (tock suggest --dry-run).
func (a *App) SuggestBatch() (suggest.Batch, error) {
	items, err := a.Inbox(suggest.InboxDays)
	if err != nil {
		return suggest.Batch{}, err
	}
	rows, err := a.st.Suggestions()
	if err != nil {
		return suggest.Batch{}, err
	}
	cached := make(map[string]bool, len(rows))
	for k := range rows {
		cached[k] = true
	}
	f, err := suggest.NewFilter(a.cfg.LLM)
	if err != nil {
		return suggest.Batch{}, err
	}
	return suggest.Prepare(items, cached, f, a.rs), nil
}

// SuggestPrepare builds the next batch and claims the hour for it. force
// ignores the hour but never the cache. The caller sends Batch.Payload with
// Model and passes the answer to SuggestSave.
func (a *App) SuggestPrepare(force bool) (suggest.Prepared, error) {
	if !a.cfg.LLM.Enabled {
		return suggest.Prepared{Status: suggest.Off}, nil
	}
	b, err := a.SuggestBatch()
	if err != nil {
		return suggest.Prepared{}, err
	}
	if len(b.Keys) == 0 {
		return suggest.Prepared{Status: suggest.Nothing}, nil
	}
	every := suggest.EveryMS
	if force {
		every = 0
	}
	ok, err := a.st.Claim(store.KeyLLMClaim, a.nowMS(), every)
	if err != nil {
		return suggest.Prepared{}, err
	}
	if !ok {
		v, _, err := a.st.State(store.KeyLLMClaim)
		last, _ := strconv.ParseInt(v, 10, 64)
		return suggest.Prepared{Status: suggest.Wait, NextMS: last + suggest.EveryMS}, err
	}
	return suggest.Prepared{Status: suggest.Ready, Batch: b, Model: a.cfg.LLM.Model}, nil
}

// SuggestSave caches the answer for every key of b, "none" included, and
// returns how many keys got a category or project.
func (a *App) SuggestSave(b suggest.Batch, model string, raw json.RawMessage) (int, error) {
	ss, err := suggest.Parse(raw, b, a.rs)
	if err != nil {
		return 0, err
	}
	now, n := a.nowMS(), 0
	rows := make([]store.Suggestion, len(ss))
	for i, s := range ss {
		rows[i] = store.Suggestion{Key: s.Key, Category: s.Category, Project: s.Project, Model: model, CreatedMS: now}
		if s.Category != "" || s.Project != "" {
			n++
		}
	}
	return n, a.st.SaveSuggestions(rows)
}

// Suggestions returns a label per inbox key: Claude's cached suggestion
// when the current rules can still apply it, in the declared spellings,
// else the local guess (see Guesses).
func (a *App) Suggestions() (map[string]classify.Want, error) {
	rows, err := a.st.Suggestions()
	if err != nil {
		return nil, err
	}
	gs, err := a.Guesses()
	if err != nil {
		return nil, err
	}
	out := make(map[string]classify.Want, len(gs))
	for k, g := range gs {
		out[k] = g.Want
	}
	for k, r := range rows {
		if w, ok := suggest.Canon(a.rs, r.Category, r.Project); ok {
			out[k] = w
		}
	}
	return out, nil
}

// GuessDays is how far back Guesses learns from labelled time.
const GuessDays = 90

// Guesses infers labels for unlabelled keys from the labelled time of the
// last GuessDays days, locally (classify.Guesses).
func (a *App) Guesses() (map[string]classify.Guess, error) {
	now := a.nowMS()
	vs, labels, err := a.labelled(now-GuessDays*24*60*60*1000, now, now)
	if err != nil {
		return nil, err
	}
	return classify.Guesses(vs, labels), nil
}

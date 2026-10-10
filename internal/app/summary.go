package app

import (
	"encoding/json"
	"time"

	"tock/internal/suggest"
)

// SummaryBatch is what tock summary would send for the calendar day of day:
// its work blocks and their filtered items. It sends nothing.
func (a *App) SummaryBatch(day time.Time) (suggest.SummaryBatch, error) {
	from := a.midnight(day)
	to := from.AddDate(0, 0, 1)
	now := a.nowMS()
	vs, labels, err := a.labelled(from.UnixMilli(), to.UnixMilli(), now)
	if err != nil {
		return suggest.SummaryBatch{}, err
	}
	d, err := a.Day(day)
	if err != nil {
		return suggest.SummaryBatch{}, err
	}
	f, err := suggest.NewFilter(a.cfg.LLM)
	if err != nil {
		return suggest.SummaryBatch{}, err
	}
	return suggest.PrepareSummary(from, d.Blocks, vs, labels, f), nil
}

// SaveSummary reads Claude's answer for b and saves it for the day.
func (a *App) SaveSummary(day time.Time, b suggest.SummaryBatch, model string, raw json.RawMessage) (suggest.Summary, error) {
	s, err := suggest.ParseSummary(raw, b.Starts)
	if err != nil {
		return s, err
	}
	return s, a.st.SaveSummary(dateKey(a.midnight(day)), model, s.Day, s.Blocks, a.nowMS())
}

// fillSummaries reads the saved summaries of date: the day's into summary,
// each block's through starts.
func (a *App) fillSummaries(date time.Time, summary *string, starts func(start int64, text string)) error {
	m, _, err := a.st.Summaries(dateKey(date))
	if err != nil {
		return err
	}
	*summary = m[0]
	for start, text := range m {
		if start != 0 {
			starts(start, text)
		}
	}
	return nil
}

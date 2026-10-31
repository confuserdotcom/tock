package app

import (
	"time"

	"tock/internal/focus"
	"tock/internal/report"
)

// routineBodies fills in the text of every routine notice that is due on
// this tick. It builds a text only behind the pure core's due-ness check, so
// an idle tick reads neither yesterday's report nor the plan file.
func (a *App) routineBodies(in *focus.Input, nowT time.Time) error {
	if focus.StartDue(*in) {
		body, err := a.StartBody(nowT)
		if err != nil {
			return err
		}
		in.StartBody = body
	}
	return nil
}

// StartBody is the start-of-day text for the calendar day of now, shared by
// the notice and tock recap start.
func (a *App) StartBody(now time.Time) (string, error) {
	y, err := a.Day(a.midnight(now).AddDate(0, 0, -1))
	if err != nil {
		return "", err
	}
	return report.StartBody(y, nil), nil
}

package app

import (
	"time"

	"tock/internal/focus"
	"tock/internal/plan"
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
	return report.StartBody(y, a.planRows(now)), nil
}

// planRows is the [plan] file's rows for the calendar day of now, or nil
// when there is no file or it cannot be read: a broken plan file never
// blocks a notice.
func (a *App) planRows(now time.Time) []plan.Row {
	if a.cfg.Plan.File == "" {
		return nil
	}
	lines, err := plan.Read(a.cfg.Plan.File)
	if err != nil {
		return nil
	}
	day := a.midnight(now)
	return plan.Rows(lines, day.Format("2006-01-02"), day.AddDate(0, 0, 1).Format("2006-01-02"))
}

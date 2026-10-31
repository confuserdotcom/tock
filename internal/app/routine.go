package app

import (
	"fmt"
	"time"

	"tock/internal/focus"
	"tock/internal/plan"
	"tock/internal/report"
)

// routineBodies fills in the text of every routine notice that is due on
// this tick. It builds a text only behind the pure core's due-ness check, so
// an idle tick reads neither yesterday's report nor the plan file.
// today builds today's report on first use and returns it again after.
// A text that cannot be built is not fatal: that notice is skipped for this
// tick (an empty body is never sent or marked as sent, so it is tried again
// on the next one) and the first failure is kept in RoutineErr.
func (a *App) routineBodies(in *focus.Input, nowT time.Time, today func() (report.Day, error)) {
	a.routineErr = nil
	skip := func(err error) {
		if a.routineErr == nil {
			a.routineErr = err
		}
	}
	if focus.StartDue(*in) {
		if body, err := a.StartBody(nowT); err != nil {
			skip(fmt.Errorf("app: start-of-day notice: %w", err))
		} else {
			in.StartBody = body
		}
	}
	if focus.MiddayDue(*in) {
		if d, err := today(); err != nil {
			skip(fmt.Errorf("app: midday notice: %w", err))
		} else {
			in.MiddayBody = report.MiddayBody(d)
		}
	}
	if focus.OverDue(*in) {
		if d, err := today(); err != nil {
			skip(fmt.Errorf("app: overworking notice: %w", err))
		} else {
			in.OverBody = report.OverBody(d, focus.OverMS(*in))
		}
	}
}

// RoutineErr is the error that kept a routine notice from being built on the
// last focus tick, nil after a tick where all were built.
func (a *App) RoutineErr() error { return a.routineErr }

// MiddayBody is the midday check-in text at that moment, shared by the
// notice and tock recap midday.
func (a *App) MiddayBody(now time.Time) (string, error) {
	d, err := a.Day(now)
	if err != nil {
		return "", err
	}
	return report.MiddayBody(d), nil
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

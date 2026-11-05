package bar

import (
	"errors"
	"fmt"
	"time"

	"tock/internal/app"
	"tock/internal/focus"
	"tock/internal/report"
	"tock/internal/store"
)

// Moment is the island a notch notice opens, read at now; ok is false for a
// topic that opens none (an up-next notice: the compact island shows it).
// unlabelledMS is today's unlabelled time, for the Label button.
func Moment(a *app.App, n store.Notice, d app.NoticeData, now time.Time, unlabelledMS int64) (Island, bool, error) {
	switch n.Topic {
	case focus.TopicFocus:
		return DriftIsland(n, d), true, nil
	case focus.TopicStarting:
		return MeetIsland(n, d), true, nil
	case focus.TopicDone:
		var det *report.SessionDetail
		var err error
		if d.Session > 0 {
			v, e := a.SessionDetail(d.Session)
			switch {
			case e == nil:
				det = &v
			case !errors.Is(e, store.ErrSessionGone):
				err = fmt.Errorf("bar: done detail: %w", e)
			}
		}
		return DoneIsland(n, d, det, now.Location(), unlabelledMS), true, err
	case focus.TopicRecap:
		day, err := a.Day(now)
		if err != nil {
			return Island{}, false, fmt.Errorf("bar: recap day: %w", err)
		}
		u, err := a.Usual(now)
		if err != nil {
			return Island{}, false, fmt.Errorf("bar: recap usual: %w", err)
		}
		return RecapIsland(n, day, u, unlabelledMS, now.Location()), true, nil
	}
	return Island{}, false, nil
}

package web

import (
	"errors"
	"net/http"
	"time"

	"tock/internal/app"
	"tock/internal/report"
)

type monthSideJSON struct {
	TrackedMS       int64   `json:"trackedMS"`
	FocusMS         int64   `json:"focusMS"`
	Workdays        int     `json:"workdays"`
	AvgScore        float64 `json:"avgScore"`
	SwitchesPerHour float64 `json:"switchesPerHour"`
}

type monthDayJSON struct {
	Date      string `json:"date"`
	TrackedMS int64  `json:"trackedMS"`
	FocusMS   int64  `json:"focusMS"`
	DeepMS    int64  `json:"deepMS"`
	LongestMS int64  `json:"longestMS"`
	Score     int    `json:"score"` // -1: not a workday
}

type trendJSON struct {
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	MS     int64  `json:"ms"`
	PrevMS int64  `json:"prevMS"`
}

type monthJSON struct {
	First      string         `json:"first"`
	Partial    bool           `json:"partial"`
	Days       []monthDayJSON `json:"days"`
	Cur        monthSideJSON  `json:"cur"`
	Prev       monthSideJSON  `json:"prev"`
	Categories []trendJSON    `json:"categories"`
}

func monthSide(s report.MonthSide) monthSideJSON {
	return monthSideJSON{s.TrackedMS, s.FocusMS, s.Workdays, s.AvgScore, s.SwitchesPerHour}
}

// month reads ?month=YYYY-MM, default this month.
func (s *Server) month(r *http.Request) (any, error) {
	now := s.now()
	day := now
	if q := r.URL.Query().Get("month"); q != "" {
		d, err := time.ParseInLocation("2006-01", q, now.Location())
		if err != nil {
			return nil, badRequest("month must be YYYY-MM")
		}
		day = d
	}
	m, err := s.a.Month(day)
	if errors.Is(err, app.ErrFutureMonth) {
		return nil, badRequest(err.Error())
	}
	if err != nil {
		return nil, err
	}
	out := monthJSON{First: m.First.Format(dateLayout), Partial: m.Partial, Days: []monthDayJSON{},
		Cur: monthSide(m.Cur), Prev: monthSide(m.Prev), Categories: []trendJSON{}}
	for i, d := range m.Days {
		out.Days = append(out.Days, monthDayJSON{Date: m.First.AddDate(0, 0, i).Format(dateLayout),
			TrackedMS: d.TrackedMS, FocusMS: d.FocusMS, DeepMS: d.DeepMS, LongestMS: d.LongestMS, Score: d.Score})
	}
	for _, c := range m.Categories {
		out.Categories = append(out.Categories, trendJSON{report.Safe(c.Name), c.Kind, c.MS, c.PrevMS})
	}
	return out, nil
}

type hoursJSON struct {
	Thin       bool    `json:"thin"`
	ActiveDays int     `json:"activeDays"`
	Grid       [][]int `json:"grid"` // [weekday, Monday first][hour]: mean focus minutes
}

// hours is the weekday x hour grid of mean focus minutes (tock trends --hours).
func (s *Server) hours(*http.Request) (any, error) {
	p, err := s.a.Profile(s.now())
	if err != nil {
		return nil, err
	}
	out := hoursJSON{Thin: p.Thin, ActiveDays: p.ActiveDays, Grid: make([][]int, 7)}
	for wd := range p.Hours {
		row := make([]int, 24)
		for h, st := range p.Hours[wd] {
			row[h] = int((st.MeanMS + 30_000) / 60_000)
		}
		out.Grid[(wd+6)%7] = row
	}
	return out, nil
}

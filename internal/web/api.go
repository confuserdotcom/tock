package web

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"time"

	"tock/internal/app"
	"tock/internal/classify"
	"tock/internal/report"
	"tock/internal/stats"
)

// Times are Unix milliseconds; durations are milliseconds. Every string
// that came from a window or URL went through report.Safe, and the page
// renders it as text only.

type nameMS struct {
	Name string `json:"name"`
	MS   int64  `json:"ms"`
}

type scoreJSON struct {
	Value           int     `json:"value"`
	Depth           float64 `json:"depth"`
	Blocks          float64 `json:"blocks"`
	Calm            float64 `json:"calm"`
	Stretch         float64 `json:"stretch"`
	Penalty         int     `json:"penalty"`
	SwitchesPerHour float64 `json:"switchesPerHour"`
	TargetMS        int64   `json:"targetMS"`
}

type statsJSON struct {
	ActiveMS     int64 `json:"activeMS"`
	DeepMS       int64 `json:"deepMS"`
	FocusMS      int64 `json:"focusMS"`
	LongestMS    int64 `json:"longestMS"`
	Switches     int   `json:"switches"`
	DistractMS   int64 `json:"distractMS"`
	LongestRunMS int64 `json:"longestRunMS"`
}

type catJSON struct {
	Name     string   `json:"name"`
	Kind     string   `json:"kind"`
	MS       int64    `json:"ms"`
	Projects []nameMS `json:"projects"`
}

type rowJSON struct {
	IDs      []int64 `json:"ids"`
	Start    int64   `json:"start"`
	End      int64   `json:"end"`
	Key      string  `json:"key"`
	App      string  `json:"app"`
	Text     string  `json:"text"`
	Category string  `json:"category"`
	Kind     string  `json:"kind"`
	Project  string  `json:"project"`
	Source   string  `json:"source"`
	Idle     bool    `json:"idle"`
	Open     bool    `json:"open"`
	Invoice  string  `json:"invoice,omitempty"` // the issued invoice that bills it
}

type goalJSON struct {
	Name   string `json:"name"`
	Text   string `json:"text"`
	AtMost bool   `json:"atMost"`
	Week   bool   `json:"week"`
	MS     int64  `json:"ms"`
	DoneMS int64  `json:"doneMS"`
	State  string `json:"state"`
	Days   int    `json:"days,omitempty"`
	Held   int    `json:"held,omitempty"`
	Streak int    `json:"streak"`
	Best   int    `json:"best"`
}

// workJSON is the day's work hours; nil without a schedule.
type workJSON struct {
	From      int64 `json:"from"`
	To        int64 `json:"to"`
	Off       bool  `json:"off"`
	OutsideMS int64 `json:"outsideMS"`
	Closed    bool  `json:"closed"`
}

type dayJSON struct {
	BilledMS   int64                `json:"billedMS,omitempty"` // time on issued invoices
	Date       string               `json:"date"`
	Today      bool                 `json:"today"`
	TrackedMS  int64                `json:"trackedMS"`
	IdleMS     int64                `json:"idleMS"`
	ByKind     map[string]int64     `json:"byKind"`
	Score      scoreJSON            `json:"score"`
	Stats      statsJSON            `json:"stats"`
	Categories []catJSON            `json:"categories"`
	HourFirst  int                  `json:"hourFirst"`
	HourLast   int                  `json:"hourLast"`
	Hours      map[string][24]int64 `json:"hours"`
	Timeline   []rowJSON            `json:"timeline"`
	Goals      []goalJSON           `json:"goals"`
	Commits    []report.NameN       `json:"commits"`
	Blocks     []blockJSON          `json:"blocks"`
	Summary    string               `json:"summary"`
	Work       *workJSON            `json:"work"`
	// Usual is the same weekday over the last weeks, nil when there are
	// fewer than two such days to compare with.
	Usual *usualJSON `json:"usual"`
	// FirstRun is true until anything at all has been recorded.
	FirstRun bool `json:"firstRun"`
	// Outages is the time trackd did not record, and why; never part of
	// the timeline or the totals.
	Outages []outageJSON `json:"outages"`
}

// outageJSON is one outage: when trackd did not record, and why.
// Partial: trackd recorded apps only (Accessibility was off), so the time
// is not part of "not recorded".
type outageJSON struct {
	From    int64  `json:"from"`
	To      int64  `json:"to"`
	Kind    string `json:"kind"`
	Why     string `json:"why"`
	Partial bool   `json:"partial"`
}

func outagesJSON(gs []stats.Gap) []outageJSON {
	out := make([]outageJSON, len(gs))
	for i, g := range gs {
		out[i] = outageJSON{From: g.From, To: g.To, Kind: g.Kind, Why: report.GapWhy(g), Partial: g.Partial()}
	}
	return out
}

type usualJSON struct {
	Days            int     `json:"days"`
	Score           int     `json:"score"`
	TrackedMS       int64   `json:"trackedMS"`
	FocusMS         int64   `json:"focusMS"`
	LongestMS       int64   `json:"longestMS"`
	DistractMS      int64   `json:"distractMS"`
	SwitchesPerHour float64 `json:"switchesPerHour"`
}

type sessionJSON struct {
	Goal      string `json:"goal"`
	StartMS   int64  `json:"startMS"`
	PlannedMS int64  `json:"plannedMS"`
}

// statusJSON is what the page's status lines show: is trackd recording,
// do the rules and config load, and the running session.
type statusJSON struct {
	Recording   bool   `json:"recording"`
	LastSeenMS  int64  `json:"lastSeenMS"`
	RulesError  string `json:"rulesError,omitempty"`
	ConfigError string `json:"configError,omitempty"`
	// Paused is why trackd records nothing (config.toml does not load).
	Paused  string       `json:"paused,omitempty"`
	Session *sessionJSON `json:"session"`
	// Focus and Billing are the groups that are on: off hides their parts
	// of the dashboard.
	Focus   bool `json:"focus"`
	Billing bool `json:"billing"`
	// Presets are the session lengths in minutes, the default first.
	Presets []int `json:"presets"`
}

type blockJSON struct {
	Start      int64    `json:"start"`
	End        int64    `json:"end"`
	Name       string   `json:"name"`
	Category   string   `json:"category"`
	Kind       string   `json:"kind"`
	ActiveMS   int64    `json:"activeMS"`
	DeepMS     int64    `json:"deepMS"`
	DistractMS int64    `json:"distractMS"`
	Top        []nameMS `json:"top"`
	Summary    string   `json:"summary"`
}

type weekDayJSON struct {
	BilledMS  int64            `json:"billedMS,omitempty"`
	Date      string           `json:"date"`
	TrackedMS int64            `json:"trackedMS"`
	ByKind    map[string]int64 `json:"byKind"`
	Future    bool             `json:"future"`
	Score     int              `json:"score"`
	DeepMS    int64            `json:"deepMS"`
}

type weekJSON struct {
	Monday       string           `json:"monday"`
	Days         []weekDayJSON    `json:"days"`
	TrackedMS    int64            `json:"trackedMS"`
	ByKind       map[string]int64 `json:"byKind"`
	Categories   []catJSON        `json:"categories"`
	Projects     []nameMS         `json:"projects"`
	Apps         []nameMS         `json:"apps"`
	Distractions []nameMS         `json:"distractions"`
	Goals        []goalJSON       `json:"goals"`
	Plan         string           `json:"plan"`
	Outages      []outageJSON     `json:"outages"`
}

type topRowJSON struct {
	Name   string `json:"name"`
	MS     int64  `json:"ms"`
	Kind   string `json:"kind"`
	Mostly string `json:"mostly"`
}

type topJSON struct {
	From     string                  `json:"from"`
	To       string                  `json:"to"`
	TotalMS  int64                   `json:"totalMS"`
	Sections map[string][]topRowJSON `json:"sections"`
}

type suggestionJSON struct {
	Label  string  `json:"label"`
	Reason string  `json:"reason,omitempty"`
	Share  float64 `json:"share,omitempty"`
}

type itemJSON struct {
	Key        string          `json:"key"`
	App        string          `json:"app"`
	MS         int64           `json:"ms"`
	Spans      int             `json:"spans"`
	Example    string          `json:"example"`
	Private    bool            `json:"private"`
	Suggestion *suggestionJSON `json:"suggestion,omitempty"`
}

type inboxJSON struct {
	Days    int        `json:"days"`
	TotalMS int64      `json:"totalMS"`
	Items   []itemJSON `json:"items"`
	// SmallKeys and SmallMS are the keys under [inbox] min_minutes, not
	// listed.
	SmallKeys int   `json:"smallKeys"`
	SmallMS   int64 `json:"smallMS"`
	// FirstRun is true until anything at all has been recorded.
	FirstRun bool `json:"firstRun"`
}

type kindName struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

type labelsJSON struct {
	Categories []kindName `json:"categories"`
	Projects   []string   `json:"projects"`
	RulesError string     `json:"rulesError,omitempty"`
}

const dateLayout = "2006-01-02"

// date reads ?date=YYYY-MM-DD as local midnight, default today.
func (s *Server) date(r *http.Request) (time.Time, error) {
	now := s.now()
	q := r.URL.Query().Get("date")
	if q == "" {
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()), nil
	}
	d, err := time.ParseInLocation(dateLayout, q, now.Location())
	if err != nil {
		return time.Time{}, badRequest("date must be YYYY-MM-DD")
	}
	return d, nil
}

func names(xs []report.NameMS) []nameMS {
	out := make([]nameMS, len(xs))
	for i, x := range xs {
		out[i] = nameMS{report.Safe(x.Name), x.MS}
	}
	return out
}

func cats(xs []report.CatRow) []catJSON {
	out := make([]catJSON, len(xs))
	for i, c := range xs {
		out[i] = catJSON{Name: c.Name, Kind: c.Kind, MS: c.MS, Projects: names(c.Projects)}
	}
	return out
}

func score(sc stats.Score) scoreJSON {
	return scoreJSON{Value: sc.Value, Depth: sc.Depth, Blocks: sc.Blocks, Calm: sc.Calm, Stretch: sc.Stretch,
		Penalty: sc.Penalty, SwitchesPerHour: sc.SwitchesPerHour, TargetMS: sc.Target.MS}
}

func (s *Server) day(r *http.Request) (any, error) {
	date, err := s.date(r)
	if err != nil {
		return nil, err
	}
	d, err := s.a.Day(date)
	if err != nil {
		return nil, err
	}
	now := s.now()
	out := dayJSON{
		Date: date.Format(dateLayout), TrackedMS: d.TrackedMS, IdleMS: d.IdleMS, ByKind: d.ByKind,
		Today:      date.Equal(time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())),
		Score:      score(d.Score),
		Stats:      statsJSON{d.Stats.ActiveMS, d.Stats.DeepMS, d.Stats.FocusMS, d.Stats.LongestMS, d.Stats.Switches, d.Stats.DistractMS, d.Stats.LongestRunMS},
		Categories: cats(d.Categories), HourFirst: d.Hours.First, HourLast: d.Hours.Last, Hours: d.Hours.MS,
		Timeline: make([]rowJSON, len(d.Timeline)), Goals: []goalJSON{}, Commits: d.Commits,
		Blocks: make([]blockJSON, len(d.Blocks)), Summary: report.Safe(d.Summary),
		Outages: outagesJSON(d.Outages),
	}
	from, to := date.UnixMilli(), date.AddDate(0, 0, 1).UnixMilli()
	billed, err := s.a.Billed(from, to)
	if err != nil {
		return nil, err
	}
	out.BilledMS = billed.MS(from, to)
	for i, row := range d.Timeline {
		out.Timeline[i] = rowJSON{IDs: row.IDs, Start: row.Start, End: row.End, Key: report.Safe(row.Key),
			App: report.Safe(row.App), Text: row.Text, Category: row.Label.Category, Kind: row.Label.Kind,
			Project: row.Label.Project, Source: string(row.Label.CategorySource), Idle: row.Idle, Open: row.Open}
		for _, id := range row.IDs {
			if n := billed.Number(id); n != "" {
				out.Timeline[i].Invoice = n
				break
			}
		}
	}
	for _, g := range d.Goals {
		out.Goals = append(out.Goals, goalJSON{Name: g.Goal.Name(), Text: g.Goal.Text(), AtMost: g.Goal.AtMost,
			Week: g.Goal.Week, MS: g.Goal.MS, DoneMS: g.DoneMS, State: g.State, Streak: g.Streak, Best: g.Best})
	}
	if d.Work.Set {
		out.Work = &workJSON{From: d.From, To: d.To, Off: d.Off, OutsideMS: d.OutsideMS, Closed: d.Closed}
	}
	h, err := s.a.Health()
	if err != nil {
		return nil, err
	}
	out.FirstRun = !h.Recorded
	u, err := s.a.Usual(date)
	if err != nil {
		return nil, err
	}
	if u.Days > 0 {
		out.Usual = &usualJSON{Days: u.Days, Score: u.Score, TrackedMS: u.TrackedMS, FocusMS: u.FocusMS,
			LongestMS: u.LongestMS, DistractMS: u.DistractMS, SwitchesPerHour: u.SwitchesPerHour}
	}
	for i, b := range d.Blocks {
		bj := blockJSON{Start: b.Start, End: b.End, Name: report.Safe(b.Name), Category: report.Safe(b.Category), Kind: b.Kind,
			ActiveMS: b.ActiveMS, DeepMS: b.DeepMS, DistractMS: b.DistractMS, Top: []nameMS{}, Summary: report.Safe(b.Summary)}
		for _, t := range b.Top {
			bj.Top = append(bj.Top, nameMS{report.Safe(t.Name), t.MS})
		}
		out.Blocks[i] = bj
	}
	return out, nil
}

func (s *Server) status(*http.Request) (any, error) {
	h, err := s.a.Health()
	if err != nil {
		return nil, err
	}
	cfg := s.a.Config()
	out := statusJSON{Recording: h.TrackdUp && !h.Paused(), LastSeenMS: h.LastSeenMS, Focus: cfg.Focus.Enabled, Billing: cfg.Billing.Enabled,
		Presets: append([]int{}, cfg.Focus.Presets...)}
	if h.Paused() {
		out.Paused = h.CapturePaused
	}
	if h.RulesErr != nil {
		out.RulesError = h.RulesErr.Error()
	}
	if h.ConfigErr != nil {
		out.ConfigError = h.ConfigErr.Error()
	}
	sess, err := s.a.RunningSession()
	if err != nil {
		return nil, err
	}
	if sess != nil {
		out.Session = &sessionJSON{Goal: report.Safe(sess.Goal), StartMS: sess.StartMS, PlannedMS: sess.PlannedMS}
	}
	return out, nil
}

func (s *Server) week(r *http.Request) (any, error) {
	date, err := s.date(r)
	if err != nil {
		return nil, err
	}
	w, err := s.a.Week(date)
	if err != nil {
		return nil, err
	}
	out := weekJSON{Monday: w.Monday.Format(dateLayout), TrackedMS: w.TrackedMS, ByKind: w.ByKind,
		Categories: cats(w.Categories), Projects: names(w.Projects), Apps: names(w.Apps),
		Distractions: names(w.Distractions), Plan: w.Plan, Goals: []goalJSON{}, Outages: outagesJSON(w.Outages)}
	billed, err := s.a.Billed(w.Monday.UnixMilli(), w.Monday.AddDate(0, 0, 7).UnixMilli())
	if err != nil {
		return nil, err
	}
	for _, d := range w.Days {
		out.Days = append(out.Days, weekDayJSON{Date: d.Date.Format(dateLayout), TrackedMS: d.TrackedMS,
			ByKind: d.ByKind, Future: d.Future, Score: d.Score.Value, DeepMS: d.Stats.DeepMS,
			BilledMS: billed.MS(d.Date.UnixMilli(), d.Date.AddDate(0, 0, 1).UnixMilli())})
	}
	for _, g := range w.Goals {
		out.Goals = append(out.Goals, goalJSON{Name: g.Goal.Name(), Text: g.Goal.Text(), AtMost: g.Goal.AtMost,
			Week: g.Goal.Week, MS: g.Goal.MS, DoneMS: g.DoneMS, State: g.State, Days: g.Days, Held: g.Held,
			Streak: g.Streak, Best: g.Best})
	}
	return out, nil
}

// top reads ?range=today|week|month or ?days=N (default 7).
func (s *Server) top(r *http.Request) (any, error) {
	now := s.now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	to := today.AddDate(0, 0, 1)
	var from time.Time
	q := r.URL.Query()
	switch q.Get("range") {
	case "today":
		from = today
	case "week":
		from = today.AddDate(0, 0, -((int(today.Weekday()) + 6) % 7))
	case "month":
		from = today.AddDate(0, 0, 1-today.Day())
	case "":
		days, err := intParam(q.Get("days"), 7, 1, 366)
		if err != nil {
			return nil, err
		}
		from = to.AddDate(0, 0, -days)
	default:
		return nil, badRequest("range must be today, week or month")
	}
	t, err := s.a.Top(from, to)
	if err != nil {
		return nil, err
	}
	out := topJSON{From: from.Format(dateLayout), To: to.AddDate(0, 0, -1).Format(dateLayout),
		TotalMS: t.TotalMS, Sections: map[string][]topRowJSON{}}
	for _, sec := range report.TopSections {
		rows := []topRowJSON{}
		for _, row := range t.Sections[sec] {
			rows = append(rows, topRowJSON{Name: report.Safe(row.Name), MS: row.MS, Kind: row.Kind, Mostly: row.Mostly})
		}
		out.Sections[sec] = rows
	}
	return out, nil
}

func intParam(v string, def, lo, hi int) (int, error) {
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < lo || n > hi {
		return 0, badRequest(fmt.Sprintf("expected a number from %d to %d", lo, hi))
	}
	return n, nil
}

func wantText(w classify.Want) string {
	if w.Project == "" {
		return w.Category
	}
	return w.Category + "/" + w.Project
}

func (s *Server) inbox(r *http.Request) (any, error) {
	days, err := intParam(r.URL.Query().Get("days"), 7, 1, 90)
	if err != nil {
		return nil, err
	}
	if err := s.a.AutoAcceptEvery(days); err != nil {
		return nil, err
	}
	items, err := s.a.Inbox(days)
	if err != nil {
		return nil, err
	}
	items, small := classify.Floor(items, s.a.Config().Inbox.MinMS)
	sugg, err := s.a.Suggestions()
	if err != nil {
		return nil, err
	}
	guesses, err := s.a.Guesses()
	if err != nil {
		return nil, err
	}
	out := inboxJSON{Days: days, Items: make([]itemJSON, len(items)), SmallKeys: len(small)}
	if len(items) == 0 {
		h, err := s.a.Health()
		if err != nil {
			return nil, err
		}
		out.FirstRun = !h.Recorded
	}
	for _, it := range small {
		out.SmallMS += it.TotalMS
	}
	for i, it := range items {
		out.TotalMS += it.TotalMS
		j := itemJSON{Key: report.Safe(it.Key), App: report.Safe(it.App), MS: it.TotalMS, Spans: it.Spans,
			Example: report.Safe(it.Example), Private: it.Private}
		if w, ok := sugg[it.Key]; ok {
			j.Suggestion = &suggestionJSON{Label: wantText(w)}
			if g, ok := guesses[it.Key]; ok && g.Want == w {
				j.Suggestion.Reason, j.Suggestion.Share = g.Reason, g.Share
			}
		}
		out.Items[i] = j
	}
	return out, nil
}

func (s *Server) labels(*http.Request) (any, error) {
	rs := s.a.Rules()
	out := labelsJSON{Categories: []kindName{}, Projects: []string{}}
	for _, c := range rs.Categories {
		out.Categories = append(out.Categories, kindName{c.Name, c.Kind})
	}
	for _, p := range rs.Projects {
		if !p.Archived {
			out.Projects = append(out.Projects, p.Name)
		}
	}
	if err := s.a.RulesErr(); err != nil {
		out.RulesError = err.Error()
	}
	return out, nil
}

// keySpans returns the spans of an inbox key. Keys travel through
// report.Safe, which leaves the keys tock builds unchanged.
func (s *Server) keySpans(key string) ([]int64, int64, error) {
	if key == "" {
		return nil, 0, badRequest("missing key")
	}
	ids, ms, err := s.a.KeySpans(key)
	if err == nil && len(ids) == 0 {
		err = httpError{http.StatusNotFound, "no spans with this key any more"}
	}
	return ids, ms, err
}

func (s *Server) want(label string) (classify.Want, error) {
	w, err := s.a.Rules().ParseWant(label)
	if err != nil {
		return w, badRequest(err.Error())
	}
	return w, nil
}

type candJSON struct {
	Rule     string `json:"rule"`
	Matches  int    `json:"matches"`
	MatchMS  int64  `json:"matchMS"`
	ThisKey  int    `json:"thisKey"`
	New      int    `json:"new"`
	NewMS    int64  `json:"newMS"`
	Fixed    int    `json:"fixed"`
	Replaces int    `json:"replaces"`
}

type proposeJSON struct {
	Spans      int        `json:"spans"`
	MS         int64      `json:"ms"`
	Label      string     `json:"label"`
	Candidates []candJSON `json:"candidates"`
}

func (s *Server) propose(r *http.Request) (any, error) {
	q := r.URL.Query()
	w, err := s.want(q.Get("label"))
	if err != nil {
		return nil, err
	}
	ids, ms, err := s.keySpans(q.Get("key"))
	if err != nil {
		return nil, err
	}
	cs, err := s.a.Propose(q.Get("key"), w)
	if err != nil {
		return nil, err
	}
	out := proposeJSON{Spans: len(ids), MS: ms, Label: report.LabelText(w.Category, w.Project), Candidates: []candJSON{}}
	for _, c := range cs {
		if c.Safe {
			out.Candidates = append(out.Candidates, candJSON{Rule: c.Rule.Describe(), Matches: c.Matches, MatchMS: c.MatchMS,
				ThisKey: c.ThisKey, New: c.New, NewMS: c.NewMS, Fixed: c.Fixed, Replaces: len(c.Replaces)})
		}
	}
	return out, nil
}

type doneJSON struct {
	Spans int    `json:"spans"`
	MS    int64  `json:"ms"`
	Label string `json:"label"`
	Note  string `json:"note,omitempty"` // the time was on an issued invoice
}

// tag labels the spans of a key, or the spans given by id, without
// learning a rule.
func (s *Server) tag(r *http.Request) (any, error) {
	var body struct {
		Key   string  `json:"key"`
		IDs   []int64 `json:"ids"`
		Label string  `json:"label"`
	}
	if err := decode(r, &body); err != nil {
		return nil, err
	}
	w, err := s.want(body.Label)
	if err != nil {
		return nil, err
	}
	ids, ms := body.IDs, int64(0)
	switch {
	case len(ids) > 0 && body.Key != "":
		return nil, badRequest("give a key or ids, not both")
	case len(ids) > 1000:
		return nil, badRequest("too many ids")
	case len(ids) == 0:
		if ids, ms, err = s.keySpans(body.Key); err != nil {
			return nil, err
		}
	}
	on, err := s.a.BilledOn(ids)
	if err != nil {
		return nil, err
	}
	if err := s.a.Tag(ids, w); err != nil {
		return nil, err
	}
	return doneJSON{len(ids), ms, report.LabelText(w.Category, w.Project), app.BilledNote(on)}, nil
}

// learn saves the candidate rule the page showed, after checking it is
// still one of the safe candidates.
func (s *Server) learn(r *http.Request) (any, error) {
	var body struct {
		Key   string `json:"key"`
		Label string `json:"label"`
		Rule  string `json:"rule"`
	}
	if err := decode(r, &body); err != nil {
		return nil, err
	}
	w, err := s.want(body.Label)
	if err != nil {
		return nil, err
	}
	cs, err := s.a.Propose(body.Key, w)
	if err != nil {
		return nil, err
	}
	i := slices.IndexFunc(cs, func(c classify.Candidate) bool { return c.Safe && c.Rule.Describe() == body.Rule })
	if i < 0 {
		return nil, httpError{http.StatusConflict, "that rule is no longer safe: look again"}
	}
	on, err := s.a.BilledOn(cs[i].IDs)
	if err != nil {
		return nil, err
	}
	if err := s.a.Learn(body.Key, cs[i], w); err != nil {
		return nil, err
	}
	return doneJSON{cs[i].Matches, cs[i].MatchMS, report.LabelText(w.Category, w.Project), app.BilledNote(on)}, nil
}

func (s *Server) ignore(r *http.Request) (any, error) {
	var body struct {
		Key  string `json:"key"`
		Undo bool   `json:"undo"`
	}
	if err := decode(r, &body); err != nil {
		return nil, err
	}
	if body.Key == "" {
		return nil, badRequest("missing key")
	}
	if err := s.a.Ignore(body.Key, !body.Undo); err != nil {
		return nil, err
	}
	return map[string]bool{"ok": true}, nil
}

// edit adds a timeline edit: an entry ("add"), a label over a range
// ("label") or a cut ("cut").
func (s *Server) edit(r *http.Request) (any, error) {
	var body struct {
		Kind  string `json:"kind"`
		From  int64  `json:"from"`
		To    int64  `json:"to"`
		Label string `json:"label"`
		Note  string `json:"note"`
	}
	if err := decode(r, &body); err != nil {
		return nil, err
	}
	var w classify.Want
	if body.Kind != classify.EditCut {
		var err error
		if w, err = s.want(body.Label); err != nil {
			return nil, err
		}
	}
	billed, err := s.a.Billed(body.From, body.To)
	if err != nil {
		return nil, err
	}
	id, err := s.a.AddEdit(body.Kind, body.From, body.To, w, body.Note)
	if err != nil {
		return nil, badRequest(err.Error())
	}
	return struct {
		ID   int64  `json:"id"`
		Note string `json:"note,omitempty"`
	}{id, app.BilledNote(billed.Numbers(body.From, body.To))}, nil
}

// undo reverses the newest change.
func (s *Server) undo(r *http.Request) (any, error) {
	if err := decode(r, &struct{}{}); err != nil {
		return nil, err
	}
	what, err := s.a.Undo()
	if errors.Is(err, app.ErrNothingToUndo) {
		return map[string]string{"what": ""}, nil
	}
	if err != nil {
		return nil, httpError{http.StatusConflict, err.Error()}
	}
	return map[string]string{"what": report.Safe(what)}, nil
}

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

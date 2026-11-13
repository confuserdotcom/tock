package web

import (
	"errors"
	"net/http"
	"slices"

	"tock/internal/app"
	"tock/internal/classify"
	"tock/internal/report"
)

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

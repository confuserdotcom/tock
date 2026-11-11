package web

import (
	"net/http"
	"strconv"

	"tock/internal/report"
)

type whyJSON struct {
	Field  string `json:"field"`
	Label  string `json:"label"`
	Source string `json:"source"`
	Text   string `json:"text"`
	Ref    string `json:"ref,omitempty"` // "rules.toml#12" when a rule set it
}

// why explains the label of one timeline entry: GET /api/why?id=&from=&to=,
// with the entry's span id and its row's start and end.
func (s *Server) why(r *http.Request) (any, error) {
	q := r.URL.Query()
	var n [3]int64
	for i, k := range []string{"id", "from", "to"} {
		v, err := strconv.ParseInt(q.Get(k), 10, 64)
		if err != nil {
			return nil, badRequest(k + ": want a number")
		}
		n[i] = v
	}
	if n[2] <= n[1] || n[2]-n[1] > 2*86_400_000 {
		return nil, badRequest("from, to: want a range of up to two days")
	}
	lines, err := s.a.Why(n[0], n[1], n[2])
	if err != nil {
		return nil, err
	}
	out := struct {
		Lines []whyJSON `json:"lines"`
	}{Lines: []whyJSON{}}
	for _, l := range lines {
		out.Lines = append(out.Lines, whyJSON{l.Field, l.Label, l.Source, report.Safe(l.Text), l.Ref})
	}
	return out, nil
}

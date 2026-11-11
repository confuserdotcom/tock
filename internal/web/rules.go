package web

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"tock/internal/app"
	"tock/internal/classify"
	"tock/internal/report"
)

type ruleJSON struct {
	File        string   `json:"file"`
	Index       int      `json:"index"`
	Fingerprint string   `json:"fingerprint"`
	Learned     bool     `json:"learned"`
	Bundle      []string `json:"bundle"`
	App         string   `json:"app"`
	Title       string   `json:"title"`
	URL         string   `json:"url"`
	Dir         string   `json:"dir"`
	Category    string   `json:"category"`
	Project     string   `json:"project"`
	Kind        string   `json:"kind"`
	MatchedMS   int64    `json:"matchedMS"`
	DecidedMS   int64    `json:"decidedMS"`
	Status      string   `json:"status"`
	Since       string   `json:"since,omitempty"` // local "2026-11-20 14:05"; "" = always
}

// rulesList is every rule in check order (learned first) with what it
// matched and decided over the last app.StatsDays days.
func (s *Server) rulesList(r *http.Request) (any, error) {
	stats, err := s.a.RuleStats(app.StatsDays)
	if err != nil {
		return nil, err
	}
	rs := s.a.Rules()
	out := struct {
		Rules []ruleJSON `json:"rules"`
		Days  int        `json:"days"`
	}{Rules: []ruleJSON{}, Days: app.StatsDays}
	for _, st := range stats {
		x := st.Rule
		b := x.Bundle
		if b == nil {
			b = []string{}
		}
		kind := ""
		if x.Category != "" {
			kind = rs.Kind(x.Category)
		} else if x.Project != "" {
			kind = rs.Kind(rs.ProjectCategory(x.Project))
		}
		out.Rules = append(out.Rules, ruleJSON{x.File, x.Index, x.Fingerprint(), x.File == classify.LearnedFile, b,
			x.App, x.Title, x.URL, x.Dir, x.Category, x.Project, kind, st.MatchedMS, st.DecidedMS, st.Status(), sinceShown(x.SinceMS)})
	}
	return out, nil
}

type ruleBody struct {
	Op          string `json:"op"`
	File        string `json:"file"`
	Index       int    `json:"index"`
	Fingerprint string `json:"fingerprint"`
	Rule        struct {
		App   string `json:"app"`
		Title string `json:"title"`
		URL   string `json:"url"`
		Dir   string `json:"dir"`
		Label string `json:"label"`
	} `json:"rule"`
}

func (b ruleBody) spec() app.RuleSpec {
	return app.RuleSpec{App: b.Rule.App, Title: b.Rule.Title, URL: b.Rule.URL, Dir: b.Rule.Dir, Label: b.Rule.Label}
}

func (b ruleBody) ref() *app.RuleRef {
	if b.Op == "add" {
		return nil
	}
	return &app.RuleRef{File: b.File, Index: b.Index, Fingerprint: b.Fingerprint}
}

// ruleErr is libErr, with a rule that changed since the page read it as a
// conflict.
func ruleErr(err error) error {
	if errors.Is(err, app.ErrRuleChanged) {
		return httpError{http.StatusConflict, "the rules changed since: reload and try again"}
	}
	return libErr(err)
}

// ruleChange adds, edits or removes one rule, undoably.
func (s *Server) ruleChange(r *http.Request) (any, error) {
	var b ruleBody
	if err := decode(r, &b); err != nil {
		return nil, err
	}
	var what string
	var fn func() error
	switch b.Op {
	case "add":
		what, fn = "added a rule", func() error { return s.a.AddRule(b.spec()) }
	case "set":
		what, fn = "changed a rule", func() error { return s.a.EditRule(*b.ref(), b.spec()) }
	case "remove":
		what, fn = "deleted a rule", func() error { return s.a.RemoveRule(*b.ref()) }
	default:
		return nil, badRequest("op must be add, set or remove")
	}
	if err := s.a.WithRulesUndo(what, fn); err != nil {
		return nil, ruleErr(err)
	}
	return libChangeJSON{what, true}, nil
}

type ruleExampleJSON struct {
	Text string `json:"text"`
	MS   int64  `json:"ms"`
	From string `json:"from"`
	To   string `json:"to"`
}

// rulePreview is what an add or an edit would do, without writing.
func (s *Server) rulePreview(r *http.Request) (any, error) {
	var b ruleBody
	if err := decode(r, &b); err != nil {
		return nil, err
	}
	if b.Op != "add" && b.Op != "set" {
		return nil, badRequest("op must be add or set")
	}
	p, err := s.a.PreviewRule(b.ref(), b.spec())
	if err != nil {
		return nil, ruleErr(err)
	}
	out := struct {
		MatchedMS int64             `json:"matchedMS"`
		ChangedMS int64             `json:"changedMS"`
		Examples  []ruleExampleJSON `json:"examples"`
	}{p.MatchedMS, p.ChangedMS, []ruleExampleJSON{}}
	for _, e := range p.Examples {
		out.Examples = append(out.Examples, ruleExampleJSON{e.Text, e.MS, e.From, e.To})
	}
	return out, nil
}

// invoice is `tock invoice` (the preview): ?project=, ?from= and ?to=
// (days, both included), ?client=, ?number=. With ?preview=1 it answers
// the totals as JSON, else the PDF marked DRAFT. It never issues; see
// invoiceIssue.
func (s *Server) invoice(r *http.Request) (any, error) {
	if !s.a.Config().Billing.Enabled {
		return nil, badRequest(app.ErrBillingOff.Error())
	}
	q := r.URL.Query()
	now := s.now()
	var days [2]time.Time
	for i, k := range []string{"from", "to"} {
		d, err := time.ParseInLocation(dateLayout, q.Get(k), now.Location())
		if err != nil {
			return nil, badRequest("?" + k + "= wants YYYY-MM-DD")
		}
		days[i] = d
	}
	if exportTooLong(days[0], days[1]) {
		return nil, badRequest("an invoice covers at most a year")
	}
	d, err := s.a.DraftInvoice(app.InvoiceSpec{Project: q.Get("project"), From: days[0], To: days[1], Client: q.Get("client"), Number: q.Get("number")})
	if err != nil {
		return nil, badRequest(err.Error())
	}
	if q.Get("preview") == "1" {
		total, _ := strconv.ParseFloat(report.FormatMinor(d.Record.TotalMinor, d.Record.MinorUnits), 64) // display only
		overlaps := d.Overlaps
		if overlaps == nil {
			overlaps = []string{}
		}
		return struct {
			Project    string   `json:"project"`
			Number     string   `json:"number"`
			Seconds    int64    `json:"seconds"`
			Total      float64  `json:"total"`
			Currency   string   `json:"currency"`
			Days       int      `json:"days"`
			ModelOnlyS int64    `json:"modelOnlyS"` // billed time only the local model labelled
			Overlaps   []string `json:"overlaps"`   // issued invoices over some of the same days
		}{d.Record.Project, d.Record.Number, d.Record.SecondsBilled, total, d.Record.Currency, len(d.Record.Lines), d.ModelOnlyS, overlaps}, nil
	}
	b, err := app.DraftPDF(d)
	if err != nil {
		return nil, err
	}
	return file{"invoice-draft-" + report.Slug(d.Record.Project) + ".pdf", "application/pdf", b}, nil
}

// invoiceIssue is tock invoice issue: the next number, stored with its PDF
// and written to the invoices folder. The body is the preview's query as
// JSON. Time only the model labelled needs confirmModel, as the CLI asks
// first; days another invoice bills need allowOverlap.
func (s *Server) invoiceIssue(r *http.Request) (any, error) {
	var body struct {
		Project      string `json:"project"`
		From         string `json:"from"`
		To           string `json:"to"`
		Client       string `json:"client"`
		Number       string `json:"number"`
		AllowOverlap bool   `json:"allowOverlap"`
		ConfirmModel bool   `json:"confirmModel"`
	}
	if err := decode(r, &body); err != nil {
		return nil, err
	}
	if !s.a.Config().Billing.Enabled {
		return nil, badRequest(app.ErrBillingOff.Error())
	}
	now := s.now()
	var days [2]time.Time
	for i, v := range []string{body.From, body.To} {
		d, err := time.ParseInLocation(dateLayout, v, now.Location())
		if err != nil {
			return nil, badRequest("from and to want YYYY-MM-DD")
		}
		days[i] = d
	}
	if exportTooLong(days[0], days[1]) {
		return nil, badRequest("an invoice covers at most a year")
	}
	spec := app.InvoiceSpec{Project: body.Project, From: days[0], To: days[1], Client: body.Client, Number: body.Number,
		AllowOverlap: body.AllowOverlap, Generator: "tock web"}
	d, err := s.a.DraftInvoice(spec)
	if err != nil {
		return nil, badRequest(err.Error())
	}
	if d.ModelOnlyS > 0 && !body.ConfirmModel {
		return nil, badRequest("some of this time only the local model labelled; look at it first, then confirm")
	}
	inv, err := s.a.IssueInvoice(spec)
	if err != nil {
		return nil, badRequest(err.Error())
	}
	total, _ := strconv.ParseFloat(report.FormatMinor(inv.TotalMinor, inv.MinorUnits), 64) // display only
	return struct {
		Number   string  `json:"number"`
		Total    float64 `json:"total"`
		Currency string  `json:"currency"`
	}{inv.Number, total, inv.Currency}, nil
}

// sinceShown is a rule's since as the Library shows it, "" when it has none.
func sinceShown(ms int64) string {
	if ms == 0 {
		return ""
	}
	return classify.SinceShort(ms)
}

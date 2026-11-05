package bar

import (
	"fmt"
	"strconv"
	"time"

	"tock/internal/app"
	"tock/internal/classify"
	"tock/internal/report"
)

// labelDays is how far back labelling in the notch looks.
const labelDays = 7

// LoadLabels reads the biggest inbox keys for the notch, each with up to
// three labels to pick: the guess first, then the day's biggest categories.
func LoadLabels(a *app.App, now time.Time) ([]Entry, error) {
	items, err := a.Inbox(labelDays)
	if err != nil {
		return nil, fmt.Errorf("bar: inbox: %w", err)
	}
	items, _ = classify.Floor(items, a.Config().Inbox.MinMS)
	sugg, err := a.Suggestions()
	if err != nil {
		return nil, fmt.Errorf("bar: suggestions: %w", err)
	}
	guesses, err := a.Guesses()
	if err != nil {
		return nil, fmt.Errorf("bar: guesses: %w", err)
	}
	d, err := a.Day(now)
	if err != nil {
		return nil, fmt.Errorf("bar: day: %w", err)
	}
	rs := a.Rules()
	kinds := map[string]string{}
	var cats []string
	for _, c := range rs.Categories {
		kinds[c.Name] = c.Kind
	}
	for _, c := range d.Categories { // by time
		if kinds[c.Name] != "" {
			cats = append(cats, c.Name)
		}
	}
	for _, c := range rs.Categories { // then the rest, in file order
		cats = append(cats, c.Name)
	}
	var out []Entry
	for _, it := range items {
		if len(out) == labelEntries {
			break
		}
		e := Entry{Key: it.Key, App: report.Safe(it.App), Title: report.Safe(it.Example), Dur: report.Dur(it.TotalMS), MS: it.TotalMS, Picked: -1}
		if it.Private || e.Title == "" {
			e.Title = report.Safe(it.Key)
		}
		e.Sub = e.App + " · " + plural(it.Spans, "entry", "entries")
		seen := map[string]bool{}
		if w, ok := sugg[it.Key]; ok && kinds[w.Category] != "" {
			o := Option{Label: WantText(w), Kind: kinds[w.Category]}
			if g, ok := guesses[it.Key]; ok && g.Want == w {
				o.Pct = int(g.Share*100 + 0.5)
			}
			e.Opts, seen[w.Category] = append(e.Opts, o), true
		}
		for _, c := range cats {
			if len(e.Opts) == 3 {
				break
			}
			if !seen[c] {
				e.Opts, seen[c] = append(e.Opts, Option{Label: c, Kind: kinds[c]}), true
			}
		}
		out = append(out, e)
	}
	return out, nil
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}

// Pick labels e with o: the first safe rule for it is learned, so the label
// reaches every past and future entry it matches; with none safe, only e's
// own spans are labelled. It says what it did.
func Pick(a *app.App, e Entry, o Option) (string, error) {
	w, err := a.Rules().ParseWant(o.Label)
	if err != nil {
		return "", fmt.Errorf("bar: label %q: %w", o.Label, err)
	}
	cs, err := a.Propose(e.Key, w)
	if err != nil {
		return "", fmt.Errorf("bar: propose: %w", err)
	}
	for _, c := range cs {
		if !c.Safe {
			continue
		}
		if err := a.Learn(e.Key, c, w); err != nil {
			return "", fmt.Errorf("bar: learn: %w", err)
		}
		return fmt.Sprintf("Rule saved: %s → %s. Labels %s, %s.", c.Rule.Describe(), WantText(w),
			plural(c.Matches, "entry", "entries"), report.Dur(c.MatchMS)), nil
	}
	ids, ms, err := a.KeySpans(e.Key)
	if err != nil {
		return "", fmt.Errorf("bar: spans: %w", err)
	}
	if err := a.Tag(ids, w); err != nil {
		return "", fmt.Errorf("bar: tag: %w", err)
	}
	return fmt.Sprintf("Labelled %s, %s → %s. No rule: one would relabel other time.", plural(len(ids), "entry", "entries"), report.Dur(ms), WantText(w)), nil
}

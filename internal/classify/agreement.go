package classify

import (
	"hash/fnv"
	"math"
	"slices"
	"strings"
)

// Agreement is how often the whole labeller (rules, then the model) gives
// the category you gave by hand, on time it never trained on. Your own
// labels are the only ground truth tock has; it is a lower bound, as you
// label by hand mostly what tock got wrong or left blank.
type Agreement struct {
	Spans, Agreed, Unlabelled  int // hand-labelled spans; those tock agrees on; those it left blank
	MS, AgreedMS, UnlabelledMS int64
	PerCategory                []AgreementRow // by hand-labelled time, most first
	Confusions                 []Confusion    // by time, most first
	Blanks                     []Blank        // what tock left blank, by Key; by time, most first
}

// Blank is time you labelled Label in windows of one Key that tock left
// unlabelled: where a rule would help most.
type Blank struct {
	Key, Label string
	MS         int64
	N          int
}

// AgreementRow is one category: time you labelled it, the part tock agreed
// on, and all the time tock guessed it (precision is AgreedMS/GuessedMS).
type AgreementRow struct {
	Category                string
	MS, AgreedMS, GuessedMS int64
	N, Agreed               int
}

// Confusion is time you labelled Label that tock labelled Guess.
type Confusion struct {
	Label, Guess string
	MS           int64
	N            int
}

// Agreement scores the labeller against the hand labels in ovs, in folds
// by Key: the model for a fold trains on every other fold, so no span is
// scored by a model that saw it, nor any span with its key. Rules are
// taken as they are; a learned rule applies only from its SinceMS, after
// the edits that taught it.
func (rs *Ruleset) Agreement(views []SpanView, ovs map[int64]Override, folds int) Agreement {
	labels := rs.LabelAll(views, ovs)
	ex := rs.TrainingPart(views, labels)
	var seed map[string][]string
	if c := rs.SeedCategory(); c != "" {
		seed = map[string][]string{c: SeedWords}
	}
	var a Agreement
	rows := map[string]*AgreementRow{}
	row := func(c string) *AgreementRow {
		r := rows[c]
		if r == nil {
			r = &AgreementRow{Category: c}
			rows[c] = r
		}
		return r
	}
	conf := map[[2]string]*Confusion{}
	blanks := map[[2]string]*Blank{}
	for _, fs := range splitFolds(views, ovs, ex, folds) {
		held := fs.held
		m := Train(MergeTraining(fs.train), seed)
		hv := make([]SpanView, len(held))
		hl := make([]Label, len(held))
		for j, i := range held {
			hv[j] = views[i]
			hl[j] = rs.Label(views[i], nil)
		}
		rs.ApplyModel(m, hv, hl)
		for j, v := range hv {
			want, got, ms := ovs[v.ID].Category, hl[j].Category, v.DurMS()
			a.Spans++
			a.MS += ms
			r := row(want)
			r.N++
			r.MS += ms
			if got != "" {
				row(got).GuessedMS += ms
			}
			switch {
			case strings.EqualFold(got, want):
				a.Agreed++
				a.AgreedMS += ms
				r.Agreed++
				r.AgreedMS += ms
			case got == "":
				a.Unlabelled++
				a.UnlabelledMS += ms
				k := [2]string{Key(v), want}
				b := blanks[k]
				if b == nil {
					b = &Blank{Key: k[0], Label: want}
					blanks[k] = b
				}
				b.MS += ms
				b.N++
			default:
				k := [2]string{want, got}
				c := conf[k]
				if c == nil {
					c = &Confusion{Label: want, Guess: got}
					conf[k] = c
				}
				c.MS += ms
				c.N++
			}
		}
	}
	for _, r := range rows {
		a.PerCategory = append(a.PerCategory, *r)
	}
	slices.SortFunc(a.PerCategory, func(x, y AgreementRow) int {
		if x.MS != y.MS {
			return cmpDesc(x.MS, y.MS)
		}
		return strings.Compare(x.Category, y.Category)
	})
	for _, c := range conf {
		a.Confusions = append(a.Confusions, *c)
	}
	slices.SortFunc(a.Confusions, func(x, y Confusion) int {
		if x.MS != y.MS {
			return cmpDesc(x.MS, y.MS)
		}
		return strings.Compare(x.Label+x.Guess, y.Label+y.Guess)
	})
	for _, b := range blanks {
		a.Blanks = append(a.Blanks, *b)
	}
	slices.SortFunc(a.Blanks, func(x, y Blank) int {
		if x.MS != y.MS {
			return cmpDesc(x.MS, y.MS)
		}
		return strings.Compare(x.Key+x.Label, y.Key+y.Label)
	})
	return a
}

// evalFold is one round of Agreement: the hand-labelled views it scores (as
// indexes) and what its model trains on.
type evalFold struct {
	held  []int
	train []TrainEntry
}

// splitFolds deals keys into folds; a fold's training never holds a key it
// scores. Folds with nothing to score are left out.
func splitFolds(views []SpanView, ovs map[int64]Override, ex []TrainEntry, folds int) []evalFold {
	folds = max(folds, 2)
	of := func(v SpanView) int {
		h := fnv.New32a()
		h.Write([]byte(Key(v)))
		return int(h.Sum32() % uint32(folds))
	}
	var out []evalFold
	for f := 0; f < folds; f++ {
		var fs evalFold
		for i, v := range views {
			if o, ok := ovs[v.ID]; ok && o.Category != "" && of(v) == f {
				fs.held = append(fs.held, i)
			}
		}
		if len(fs.held) == 0 {
			continue
		}
		for _, e := range ex {
			if of(e.View) != f {
				fs.train = append(fs.train, e)
			}
		}
		out = append(out, fs)
	}
	return out
}

func cmpDesc(x, y int64) int {
	if x > y {
		return -1
	}
	return 1
}

// Wilson is the 95 % Wilson score interval of k successes in n trials;
// 0..1 when n is 0.
func Wilson(k, n int) (lo, hi float64) {
	if n == 0 {
		return 0, 1
	}
	const z = 1.959964
	p, nf := float64(k)/float64(n), float64(n)
	d := 1 + z*z/nf
	c := p + z*z/(2*nf)
	w := z * math.Sqrt(p*(1-p)/nf+z*z/(4*nf*nf))
	return (c - w) / d, (c + w) / d
}

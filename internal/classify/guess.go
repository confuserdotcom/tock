package classify

import (
	"strings"

	"tock/internal/span"
)

// Guess is a label for an inbox key inferred from labelled history.
type Guess struct {
	Want
	Share  float64 // the winning category's (else project's) share of the vote
	Reason string  // the strongest evidence: GuessSite, GuessApp, GuessTitle or GuessNear
}

// Guess reasons.
const (
	GuessSite  = "same site"
	GuessApp   = "same app"
	GuessTitle = "similar titles"
	GuessNear  = "used alongside"
)

// Guess thresholds and weights.
const (
	GuessMinShare = 0.6
	GuessMinMS    = 5 * 60_000 // evidence: labelled time behind the winner
	guessNearMS   = 2 * 60_000 // a neighbour this close counts as context

	weightSite, weightApp, weightTitle, weightNear = 3, 2, 1, 1
)

type vote struct {
	ms       map[string]int64 // weighted, by name
	evidence map[string]int64 // unweighted, by name
	byReason map[string]map[string]int64
}

func newVote() *vote {
	return &vote{ms: map[string]int64{}, evidence: map[string]int64{}, byReason: map[string]map[string]int64{}}
}

func (v *vote) add(name, reason string, weight, ms int64) {
	if name == "" || ms <= 0 {
		return
	}
	v.ms[name] += weight * ms
	v.evidence[name] += ms
	if v.byReason[name] == nil {
		v.byReason[name] = map[string]int64{}
	}
	v.byReason[name][reason] += weight * ms
}

// winner returns the top name when it holds GuessMinShare of the vote and
// GuessMinMS of evidence.
func (v *vote) winner() (name string, share float64, reason string) {
	var total, best int64
	for n, ms := range v.ms {
		total += ms
		if ms > best || (ms == best && n < name) {
			name, best = n, ms
		}
	}
	if total == 0 || v.evidence[name] < GuessMinMS {
		return "", 0, ""
	}
	if share = float64(best) / float64(total); share < GuessMinShare {
		return "", 0, ""
	}
	var rbest int64
	for _, r := range []string{GuessSite, GuessApp, GuessTitle, GuessNear} {
		if ms := v.byReason[name][r]; ms > rbest {
			reason, rbest = r, ms
		}
	}
	return name, share, reason
}

// Guesses infers labels for the keys of unlabelled, non-idle, non-private
// views from the labelled ones, weighted by time:
//   - the same URL host (x3) for keys with a URL;
//   - the same bundle (x2) for keys without one;
//   - shared title words (x1), each word shared by at most a quarter of the
//     labelled views;
//   - the label of the views directly before and after, within 2 minutes,
//     when both share it (x1, for the unlabelled view's own time).
//
// Category and project are voted separately. A field is guessed when its
// winner holds 60 % of the vote and 5 minutes of evidence. Keys with
// nothing guessed are absent. views must be in time order; labels[i] is the
// label of views[i].
func Guesses(views []SpanView, labels []Label) map[string]Guess {
	keys := make([]string, len(views))
	unl := map[string][]int{}
	for i := range views {
		v := &views[i]
		if v.BundleID == span.IdleBundleID {
			continue
		}
		keys[i] = Key(*v)
		if !v.Private && labels[i].Category == "" && labels[i].Project == "" {
			unl[keys[i]] = append(unl[keys[i]], i)
		}
	}
	if len(unl) == 0 {
		return nil
	}

	// Indexes over the labelled views.
	byHost := map[string][]int{}
	byBundle := map[string][]int{}
	byWord := map[string][]int{}
	labelled := 0
	for i := range views {
		v := &views[i]
		if keys[i] == "" || (labels[i].Category == "" && labels[i].Project == "") {
			continue
		}
		labelled++
		if h := hostOf(v); h != "" {
			byHost[h] = append(byHost[h], i)
		} else {
			byBundle[v.BundleID] = append(byBundle[v.BundleID], i)
		}
		if v.Title != nil && !v.Private {
			for w := range words(Clean(*v.Title)) {
				byWord[w] = append(byWord[w], i)
			}
		}
	}
	if labelled == 0 {
		return nil
	}

	out := map[string]Guess{}
	for key, idx := range unl {
		cat, proj := newVote(), newVote()
		add := func(j int, reason string, weight int64) {
			ms := views[j].DurMS()
			cat.add(labels[j].Category, reason, weight, ms)
			proj.add(labels[j].Project, reason, weight, ms)
		}
		first := &views[idx[0]]
		if h := hostOf(first); h != "" {
			for _, j := range byHost[h] {
				add(j, GuessSite, weightSite)
			}
		} else {
			for _, j := range byBundle[first.BundleID] {
				add(j, GuessApp, weightApp)
			}
		}
		ws := map[string]bool{}
		for _, i := range idx {
			if views[i].Title != nil {
				for w := range words(Clean(*views[i].Title)) {
					ws[w] = true
				}
			}
		}
		seen := map[int]bool{}
		for w := range ws {
			if js := byWord[w]; len(js)*4 <= labelled {
				for _, j := range js {
					if !seen[j] {
						seen[j] = true
						add(j, GuessTitle, weightTitle)
					}
				}
			}
		}
		for _, i := range idx {
			// Context counts, for as long as the unlabelled view lasted,
			// when the views on both sides share the label.
			j, k := neighbour(views, keys, i, -1), neighbour(views, keys, i, 1)
			if j < 0 || k < 0 {
				continue
			}
			if c := labels[j].Category; c == labels[k].Category {
				cat.add(c, GuessNear, weightNear, views[i].DurMS())
			}
			if p := labels[j].Project; p == labels[k].Project {
				proj.add(p, GuessNear, weightNear, views[i].DurMS())
			}
		}

		var g Guess
		c, cs, cr := cat.winner()
		p, ps, pr := proj.winner()
		g.Category, g.Project = c, p
		switch {
		case c != "":
			g.Share, g.Reason = cs, cr
		case p != "":
			g.Share, g.Reason = ps, pr
		default:
			continue
		}
		out[key] = g
	}
	return out
}

// hostOf is v's URL host without "www.", or "".
func hostOf(v *SpanView) string {
	if v.Private || v.URL == nil || *v.URL == "" {
		return ""
	}
	h, _, _ := strings.Cut(strings.TrimPrefix(*v.URL, "www."), "/")
	return strings.ToLower(h)
}

// neighbour is the index of the nearest non-idle view before (dir -1) or
// after (dir 1) view i with another key, within guessNearMS; else -1.
func neighbour(views []SpanView, keys []string, i, dir int) int {
	for j := i + dir; j >= 0 && j < len(views); j += dir {
		if keys[j] == "" {
			continue // idle
		}
		gap := views[i].Start - views[j].End
		if dir > 0 {
			gap = views[j].Start - views[i].End
		}
		if gap > guessNearMS {
			return -1
		}
		if keys[j] != keys[i] {
			return j
		}
	}
	return -1
}

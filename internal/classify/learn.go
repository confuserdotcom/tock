package classify

import (
	"errors"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"tock/internal/span"
)

var (
	// ErrNoSpans means no span in history has the key being tagged.
	ErrNoSpans = errors.New("no spans with this key")
	// ErrPrivateKey means the key groups private spans, which no rule can
	// target; tag them with --once instead.
	ErrPrivateKey = errors.New("private windows cannot be learned; use tock tag --once")
)

// Want is the label `tock tag` asks for. Empty fields are left alone.
type Want struct {
	Category, Project string
}

// Candidate is a proposed learned rule and what it would do to history.
type Candidate struct {
	Rule Rule

	Matches   int   // spans the rule matches in history
	MatchMS   int64 // their total time
	ThisKey   int   // matches that have the tagged key
	NewMS     int64 // time of other matches that gain a label
	New       int   // other matches that gain a label
	Relabeled int   // matches whose rule-given label would change
	// Fixed counts matches outside the key whose label comes from a learned
	// rule that labels the key itself differently: the mistake being fixed.
	Fixed int
	// Replaces are the 0-based positions in learned.toml of the fixed rules
	// this rule covers completely; Learn removes them.
	Replaces []int
	Safe     bool // Relabeled == 0
}

// unicodeWord matches a whole word without relying on RE2's ASCII-only \b.
const wordLeft, wordRight = `(^|[^\pL\pN])`, `([^\pL\pN]|$)`

// stopwords are frequent English and French words that never make a useful
// title rule.
var stopwords = map[string]bool{
	"the": true, "and": true, "for": true, "with": true, "from": true, "that": true,
	"this": true, "your": true, "you": true, "are": true, "not": true, "new": true,
	"les": true, "des": true, "une": true, "pour": true, "dans": true, "avec": true,
	"sur": true, "par": true, "est": true, "pas": true, "qui": true, "que": true,
}

// words returns the distinct lowercase words of s: letter/digit runs of at
// least 3 runes that are not all digits and not stopwords.
func words(s string) map[string]bool {
	out := map[string]bool{}
	for _, w := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		if utf8.RuneCountInString(w) < 3 || stopwords[w] || strings.IndexFunc(w, unicode.IsLetter) < 0 {
			continue
		}
		out[w] = true
	}
	return out
}

// Propose returns the learned-rule candidates for key, most general first.
// Every candidate matches all spans of the key by construction (the key's
// host, bundles, a word all its titles share, or its one shared title).
// Callers offer the first Safe one; "more specific" moves to the next Safe
// one.
func (rs *Ruleset) Propose(key string, want Want, history []SpanView, overrides map[int64]Override) ([]Candidate, error) {
	var target []int
	for i := range history {
		if history[i].BundleID != span.IdleBundleID && Key(history[i]) == key {
			target = append(target, i)
		}
	}
	if len(target) == 0 {
		return nil, ErrNoSpans
	}
	first := history[target[0]]
	if first.Private {
		return nil, ErrPrivateKey
	}

	bundles := bundlesOf(history, target)
	var rules []Rule
	if first.URL != nil && *first.URL != "" {
		host, seg := splitURLKey(urlKey(*first.URL))
		hostRe := `^(www\.)?` + regexp.QuoteMeta(host) + `(/|$)`
		rules = append(rules, Rule{URL: hostRe})
		if seg != "" {
			rules = append(rules, Rule{URL: `^(www\.)?` + regexp.QuoteMeta(host+"/"+seg) + `(/|$)`})
		}
		rules = append(rules, Rule{Bundle: bundles, URL: hostRe})
	} else {
		rules = append(rules, Rule{Bundle: bundles})
		if w := distinctiveWord(history, target); w != "" {
			rules = append(rules, Rule{Bundle: bundles, Title: wordLeft + regexp.QuoteMeta(w) + wordRight})
		}
		if t, ok := sharedClean(history, target); ok {
			rules = append(rules, Rule{Bundle: bundles, Title: `^` + regexp.QuoteMeta(t) + `$`})
		}
	}

	inTarget := make(map[int]bool, len(target))
	for _, i := range target {
		inTarget[i] = true
	}
	labels := make([]Label, len(history))
	for i := range history {
		var o *Override
		if ov, ok := overrides[history[i].ID]; ok {
			o = &ov
		}
		labels[i] = rs.Label(history[i], o)
	}
	// fixing are the learned rules that give the key a label other than want.
	fixing := map[string]bool{}
	for _, i := range target {
		l := labels[i]
		if want.Category != "" && l.CategorySource == SourceLearned && l.Category != want.Category {
			fixing[l.CategoryRule] = true
		}
		if want.Project != "" && l.ProjectSource == SourceLearned && l.Project != want.Project {
			fixing[l.ProjectRule] = true
		}
	}
	var out []Candidate
	for _, r := range rules {
		r.File, r.Category, r.Project = LearnedFile, want.Category, want.Project
		if err := r.compile(); err != nil {
			return nil, err
		}
		c := Candidate{Rule: r}
		for i := range history {
			v := &history[i]
			if v.BundleID == span.IdleBundleID {
				continue
			}
			clean := ""
			if v.Title != nil {
				clean = Clean(*v.Title)
			}
			if !r.matches(v, clean) {
				continue
			}
			c.Matches++
			c.MatchMS += v.DurMS()
			if inTarget[i] {
				c.ThisKey++
				continue
			}
			cur := labels[i]
			relabel, gain := effect(cur.Category, cur.CategorySource, want.Category)
			r2, g2 := effect(cur.Project, cur.ProjectSource, want.Project)
			catFix, projFix := relabel && fixing[cur.CategoryRule], r2 && fixing[cur.ProjectRule]
			switch {
			case (relabel && !catFix) || (r2 && !projFix):
				c.Relabeled++
			case catFix || projFix:
				c.Fixed++
			case gain || g2:
				c.New++
				c.NewMS += v.DurMS()
			}
		}
		c.Safe = c.Relabeled == 0
		c.Replaces = rs.covered(r, fixing, history)
		out = append(out, c)
	}
	return out, nil
}

// effect says what setting a field to want does to a span whose field is cur
// (from src): relabel when a rule gave it a different value, gain when it was
// empty. Manual values never change, so they have no effect.
func effect(cur string, src Source, want string) (relabel, gain bool) {
	if want == "" || src == SourceManual {
		return false, false
	}
	if cur == "" {
		return false, true
	}
	return cur != want, false
}

func bundlesOf(history []SpanView, idx []int) []string {
	seen := map[string]bool{}
	var out []string
	for _, i := range idx {
		if b := history[i].BundleID; !seen[b] {
			seen[b] = true
			out = append(out, b)
		}
	}
	sort.Strings(out)
	return out
}

func splitURLKey(k string) (host, seg string) {
	host, seg, _ = strings.Cut(k, "/")
	return host, seg
}

// sharedClean returns the Clean title when every target span has the same
// non-empty one.
func sharedClean(history []SpanView, target []int) (string, bool) {
	t := ""
	for n, i := range target {
		if history[i].Title == nil {
			return "", false
		}
		c := Clean(*history[i].Title)
		if c == "" || (n > 0 && c != t) {
			return "", false
		}
		t = c
	}
	return t, true
}

// distinctiveWord picks the word every target title contains that occurs in
// the fewest history titles; ties go to the longer word, then alphabetical.
func distinctiveWord(history []SpanView, target []int) string {
	var common map[string]bool
	for _, i := range target {
		if history[i].Title == nil {
			return ""
		}
		ws := words(Clean(*history[i].Title))
		if common == nil {
			common = ws
			continue
		}
		for w := range common {
			if !ws[w] {
				delete(common, w)
			}
		}
	}
	if len(common) == 0 {
		return ""
	}
	count := map[string]int{}
	for i := range history {
		if history[i].Title == nil {
			continue
		}
		for w := range words(Clean(*history[i].Title)) {
			if common[w] {
				count[w]++
			}
		}
	}
	cands := make([]string, 0, len(common))
	for w := range common {
		cands = append(cands, w)
	}
	sort.Slice(cands, func(a, b int) bool {
		wa, wb := cands[a], cands[b]
		if count[wa] != count[wb] {
			return count[wa] < count[wb]
		}
		if la, lb := utf8.RuneCountInString(wa), utf8.RuneCountInString(wb); la != lb {
			return la > lb
		}
		return wa < wb
	})
	return cands[0]
}

// covered returns the 0-based learned positions of the fixing rules whose
// every non-idle match in history r also matches.
func (rs *Ruleset) covered(r Rule, fixing map[string]bool, history []SpanView) []int {
	var out []int
	for n := range rs.Learned {
		old := &rs.Learned[n]
		if !fixing[old.Ref()] {
			continue
		}
		all := true
		for i := range history {
			v := &history[i]
			if v.BundleID == span.IdleBundleID {
				continue
			}
			clean := ""
			if v.Title != nil {
				clean = Clean(*v.Title)
			}
			if old.matches(v, clean) && !r.matches(v, clean) {
				all = false
				break
			}
		}
		if all {
			out = append(out, n)
		}
	}
	return out
}

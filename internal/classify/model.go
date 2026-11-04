package classify

import (
	"math"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"tock/internal/span"
)

// SourceModel marks a category the local model chose (see ApplyModel).
const SourceModel Source = "model"

// SourceScreen marks a category the local model chose only thanks to the
// words read on screen (see SpanView.Screen).
const SourceScreen Source = "screen"

// screenWeight is the say of a word read on screen next to a title word:
// the screen holds more noise than the title.
const screenWeight = 0.5

// Model thresholds: the share of the posterior a category needs, and the
// fewest known features behind it.
const (
	ModelFillMin     = 0.8  // to fill a view no rule labelled
	ModelOverrideMin = 0.85 // to replace a generic rule's label, on the title alone
	ModelMinFeatures = 2    // known features behind either
	stemRunes        = 6    // words are cut to this many letters
	modelSmooth      = 2.0  // weight of the overall prior in a site's, app's or word's
	seedWeight       = 2.0  // weight of one seed word
)

// Example is one labelled view to learn from. Blind names what labelled it
// without looking at the title: a rule's Ref() when the rule names only an
// app or a site, RemoteBlind of the view and category for another machine's
// label, "" otherwise.
// Such a label weighs less, and cannot back itself up (see ApplyModel).
type Example struct {
	View     SpanView
	Category string
	Weight   float64
	Blind    string
}

// RemoteBlind is Example.Blind for another machine's label of v: its
// machine, app and category, which stand in for the rule that set it.
func RemoteBlind(v SpanView, category string) string {
	return "remote:" + v.App + "\x00" + category
}

// Model is a naive Bayes classifier trained on the labels you already have:
// how likely each category is for the view's site (else its app), times
// how likely each is to have the view's title words. It is pure: nothing
// leaves the machine.
type Model struct {
	cats    []string
	prior   map[string]float64            // category -> weight
	all     float64                       // weight of every example
	meta    map[string]map[string]float64 // "b:"/"h:" feature -> category -> weight
	metaAll map[string]float64            // feature -> weight
	word    map[string]map[string]float64 // title word -> category -> weight
	wordAll map[string]float64            // title word -> weight
	wordN   map[string]int                // title word -> examples with it
	// blind and blindN are the part of word and wordN each blind labeller
	// taught (see Example).
	blind  map[string]map[string]map[string]float64
	blindN map[string]map[string]int
}

// minSupport is how many examples a title word needs before it counts: a
// word seen once says more about that one window than about a category.
const minSupport = 2

var accents = strings.NewReplacer(
	"à", "a", "â", "a", "ä", "a", "á", "a", "é", "e", "è", "e", "ê", "e", "ë", "e",
	"î", "i", "ï", "i", "í", "i", "ô", "o", "ö", "o", "ó", "o", "ù", "u", "û", "u",
	"ü", "u", "ú", "u", "ç", "c", "ñ", "n", "œ", "oe", "æ", "ae", "ÿ", "y",
)

// stems are the distinct words of s, lowercased, accents folded, without a
// plural s and cut to stemRunes letters, sorted.
func stems(s string) []string {
	var out []string
	for w := range words(accents.Replace(strings.ToLower(s))) {
		if n := len(w); n > 3 && w[n-1] == 's' {
			w = w[:n-1]
		}
		if utf8.RuneCountInString(w) > stemRunes {
			w = string([]rune(w)[:stemRunes])
		}
		out = append(out, w)
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// Features are a view's app and URL host ("b:", "h:"), and its title words
// without the app's own suffix (see stems). A private view has no title
// words and no host.
func Features(v SpanView) (meta, title []string) {
	meta = []string{"b:" + v.BundleID}
	if h := hostOf(&v); h != "" {
		meta = append(meta, "h:"+h)
	}
	if v.Private || v.Title == nil {
		return meta, nil
	}
	app := v.App
	if IsRemote(v) {
		_, app, _ = strings.Cut(app, " · ")
	}
	for _, t := range stems(stripAppSuffix(Clean(*v.Title), app)) {
		if !platforms[t] {
			title = append(title, t)
		}
	}
	return meta, title
}

// feature is one content word of a view and its weight.
type feature struct {
	t string
	w float64
}

// content are a view's title words (weight 1) and the words read on its
// screen that are not in the title (screenWeight), with the same stems so
// what titles taught applies to screen text. A private view has none.
func content(v SpanView) []feature {
	_, title := Features(v)
	out := make([]feature, 0, len(title))
	for _, t := range title {
		out = append(out, feature{t, 1})
	}
	if v.Private || len(v.Screen) == 0 {
		return out
	}
	for _, t := range stems(strings.Join(v.Screen, " ")) {
		if !platforms[t] && !slices.Contains(title, t) {
			out = append(out, feature{t, screenWeight})
		}
	}
	return out
}

// platforms are the stems of apps and sites that show in titles ("… -
// YouTube - Chromium"): they say where a window is, not what it is about,
// and the app and host features already carry that.
var platforms = map[string]bool{}

func init() {
	for _, w := range []string{"youtube", "google", "chrome", "chromium", "firefox", "safari", "arc", "brave",
		"github", "gitlab", "reddit", "twitter", "instagram", "facebook", "tiktok", "twitch", "netflix",
		"spotify", "duckduckgo", "bing", "wikipedia", "stackoverflow", "obsidian", "notion", "discord",
		"slack", "whatsapp", "telegram", "gmail", "outlook", "copilot", "chatgpt", "claude", "mozilla"} {
		for _, t := range stems(w) {
			platforms[t] = true
		}
	}
}

// Train builds a model from the examples, plus seed words per category
// that count as one small example each.
func Train(ex []Example, seed map[string][]string) *Model {
	m := &Model{prior: map[string]float64{}, meta: map[string]map[string]float64{}, metaAll: map[string]float64{},
		word: map[string]map[string]float64{}, wordAll: map[string]float64{},
		blind: map[string]map[string]map[string]float64{}, wordN: map[string]int{}, blindN: map[string]map[string]int{}}
	cat := func(c string) {
		if _, ok := m.prior[c]; !ok {
			m.cats = append(m.cats, c)
			m.prior[c] = 0
		}
	}
	addWords := func(c string, ws []feature, w float64, n int) {
		for _, f := range ws {
			t := f.t
			if m.word[t] == nil {
				m.word[t] = map[string]float64{}
			}
			m.word[t][c] += w * f.w
			m.wordAll[t] += w * f.w
			m.wordN[t] += n
		}
	}
	for _, e := range ex {
		if e.Category == "" || e.Weight <= 0 {
			continue
		}
		cat(e.Category)
		m.prior[e.Category] += e.Weight
		m.all += e.Weight
		meta, _ := Features(e.View)
		title := content(e.View)
		for _, f := range meta {
			if m.meta[f] == nil {
				m.meta[f] = map[string]float64{}
			}
			m.meta[f][e.Category] += e.Weight
			m.metaAll[f] += e.Weight
		}
		addWords(e.Category, title, e.Weight, 1)
		if e.Blind != "" {
			b := m.blind[e.Blind]
			if b == nil {
				b = map[string]map[string]float64{}
				m.blind[e.Blind], m.blindN[e.Blind] = b, map[string]int{}
			}
			for _, f := range title {
				t := f.t
				if b[t] == nil {
					b[t] = map[string]float64{}
				}
				b[t][e.Category] += e.Weight * f.w
				m.blindN[e.Blind][t]++
			}
		}
	}
	for c, ws := range seed {
		cat(c)
		for _, w := range ws {
			for _, t := range stems(w) {
				addWords(c, []feature{{t, 1}}, seedWeight, minSupport)
			}
		}
	}
	slices.Sort(m.cats)
	return m
}

// Predict is the most likely category of v, its share of the posterior and
// how many of v's site and title features the model knows (the app alone
// says little).
func (m *Model) Predict(v SpanView) (string, float64, int) {
	r, n := m.Rank(v, false, "")
	if len(r) == 0 {
		return "", 0, 0
	}
	return r[0].Category, r[0].P, n
}

// PredictTitle is Predict on the title words alone, against the overall
// odds, and without what blind taught: what the title says, whatever the
// app and whatever the label it has.
func (m *Model) PredictTitle(v SpanView, blind string) (string, float64, int) {
	r, n := m.Rank(v, true, blind)
	if len(r) == 0 {
		return "", 0, 0
	}
	return r[0].Category, r[0].P, n
}

// Scored is a category and its share of the posterior.
type Scored struct {
	Category string
	P        float64
}

// Rank is every category by its share of the posterior for v, best first,
// and how many of v's site and title features the model knows. The prior
// is the category's share of the time on the view's site, else its app,
// else everywhere; titleOnly keeps it at everywhere, so the title alone
// decides against the overall odds. The words blind (see Example) taught
// are left out.
func (m *Model) Rank(v SpanView, titleOnly bool, blind string) ([]Scored, int) {
	if len(m.cats) == 0 {
		return nil, 0
	}
	meta, _ := Features(v)
	drop := m.blind[blind]
	var known []feature
	for _, f := range content(v) {
		if m.knows(f.t, blind) {
			known = append(known, f)
		}
	}
	n := len(known)
	base := ""
	if !titleOnly {
		for _, f := range meta { // the host, when there is one, comes last and wins
			if m.meta[f] != nil {
				base = f
			}
		}
		if strings.HasPrefix(base, "h:") {
			n++
		}
	}
	if n == 0 {
		return nil, 0
	}
	out := make([]Scored, len(m.cats))
	top := math.Inf(-1)
	for i, c := range m.cats {
		prior := (m.prior[c] + 1) / (m.all + float64(len(m.cats)))
		p := prior
		if base != "" {
			p = (m.meta[base][c] + modelSmooth*prior) / (m.metaAll[base] + modelSmooth)
		}
		s := math.Log(p)
		// Each word's say: how much likelier c is among the views with that
		// word than overall. Smoothing toward the prior keeps one stray word
		// from making a small category win.
		for _, f := range known {
			t := f.t
			w, all := m.word[t][c]-drop[t][c], m.wordAll[t]-sum(drop[t])
			s += f.w * math.Log((w+modelSmooth*prior)/(all+modelSmooth)/prior)
		}
		out[i] = Scored{c, s}
		top = max(top, s)
	}
	var sum float64
	for i := range out {
		out[i].P = math.Exp(out[i].P - top)
		sum += out[i].P
	}
	for i := range out {
		out[i].P /= sum
	}
	slices.SortStableFunc(out, func(a, b Scored) int {
		switch {
		case a.P > b.P:
			return -1
		case a.P < b.P:
			return 1
		}
		return 0
	})
	return out, n
}

// Because is v's known title words by how much each makes category likelier
// than overall (log odds; negative is against it), strongest first,
// leaving out what blind taught.
func (m *Model) Because(v SpanView, category, blind string) []Scored {
	drop := m.blind[blind]
	prior := (m.prior[category] + 1) / (m.all + float64(len(m.cats)))
	var out []Scored
	for _, f := range content(v) {
		t := f.t
		if !m.knows(t, blind) {
			continue
		}
		all := m.wordAll[t] - sum(drop[t])
		w := m.word[t][category] - drop[t][category]
		out = append(out, Scored{t, f.w * math.Log((w+modelSmooth*prior)/(all+modelSmooth)/prior)})
	}
	slices.SortStableFunc(out, func(a, b Scored) int {
		switch {
		case math.Abs(a.P) > math.Abs(b.P):
			return -1
		case math.Abs(a.P) < math.Abs(b.P):
			return 1
		}
		return 0
	})
	return out
}

// backers is how many of v's title words make category likelier.
func (m *Model) backers(v SpanView, category, blind string) int {
	n := 0
	for _, w := range m.Because(v, category, blind) {
		if w.P > 0 {
			n++
		}
	}
	return n
}

// knows reports whether title word t has minSupport examples besides what
// blind taught.
func (m *Model) knows(t, blind string) bool {
	return m.word[t] != nil && m.wordN[t]-m.blindN[blind][t] >= minSupport && m.wordAll[t]-sum(m.blind[blind][t]) > 1e-9
}

func sum(m map[string]float64) float64 {
	var t float64
	for _, v := range m {
		t += v
	}
	return t
}

// Categories is what the model knows, sorted.
func (m *Model) Categories() []string { return m.cats }

// Source weights of training examples: what you said yourself counts most,
// a rule that only names an app or a site least.
var trainWeight = map[Source]float64{
	SourceManual: 3, SourceLearned: 2, SourceModule: 2, SourceRule: 1.5, SourceKept: 1,
}

const genericWeight = 0.5

// TrainingSet is one example per key and category among the labelled
// views, weighted by where the label came from and, gently, by time.
// Remote labels count as rules of the other machine.
func (rs *Ruleset) TrainingSet(views []SpanView, labels []Label) []Example {
	return MergeTraining(rs.TrainingPart(views, labels))
}

// TrainEntry is what one stretch of the timeline says about one key and
// category: the first view of it, the strongest label weight, the time,
// and whether a label read the title (else Blind is the labeller).
type TrainEntry struct {
	Key      string // Key of the view and the category
	View     SpanView
	Category string
	W        float64
	MS       int64
	Blind    string
	Seen     bool
}

// TrainingPart is the entries of views, in order of first appearance. The
// parts of consecutive stretches, merged in order, are the part of the
// whole (see MergeTraining), so a closed day's part can be kept.
func (rs *Ruleset) TrainingPart(views []SpanView, labels []Label) []TrainEntry {
	var out []TrainEntry
	by := map[string]int{}
	generic := rs.genericRefs()
	for i := range views {
		v, l := &views[i], &labels[i]
		if v.BundleID == span.IdleBundleID || strings.HasPrefix(v.BundleID, "tock.") || l.Category == "" {
			continue
		}
		w, ok := trainWeight[l.CategorySource]
		blind := ""
		switch {
		case IsRemote(*v) && l.CategorySource == SourceManual:
			w, ok, blind = 1, true, RemoteBlind(*v, l.Category)
		case !ok:
			continue
		case (l.CategorySource == SourceRule || l.CategorySource == SourceLearned) && generic[l.CategoryRule]:
			w, blind = genericWeight, l.CategoryRule
		}
		k := Key(*v) + "\x00" + l.Category
		j, ok := by[k]
		if !ok {
			j = len(out)
			by[k] = j
			out = append(out, TrainEntry{Key: k, View: *v, Category: l.Category, Blind: blind})
		}
		e := &out[j]
		e.W, e.MS, e.Seen = max(e.W, w), e.MS+v.DurMS(), e.Seen || blind == ""
	}
	return out
}

// MergeTraining is the examples of parts taken in order: per key and
// category the first view and labeller, the strongest weight and the
// whole time.
func MergeTraining(parts ...[]TrainEntry) []Example {
	var all []TrainEntry
	by := map[string]int{}
	for _, part := range parts {
		for _, p := range part {
			j, ok := by[p.Key]
			if !ok {
				by[p.Key] = len(all)
				all = append(all, p)
				continue
			}
			e := &all[j]
			e.W, e.MS, e.Seen = max(e.W, p.W), e.MS+p.MS, e.Seen || p.Seen
		}
	}
	out := make([]Example, 0, len(all))
	for _, e := range all {
		x := Example{View: e.View, Category: e.Category, Weight: e.W * (1 + math.Log2(1+float64(e.MS)/60_000))}
		if !e.Seen {
			x.Blind = e.Blind
		}
		out = append(out, x)
	}
	return out
}

// genericRefs are the rules that name no title or directory: they label a
// whole app or site, whatever is in it.
func (rs *Ruleset) genericRefs() map[string]bool {
	out := map[string]bool{}
	for _, set := range [][]Rule{rs.Learned, rs.Rules} {
		for _, r := range set {
			if r.Title == "" && r.Dir == "" {
				out[r.Ref()] = true
			}
		}
	}
	return out
}

// ApplyModel labels with m, in place:
//   - a view no rule labelled (or that a call labelled only because
//     nothing else did) gets m's category when it holds ModelFillMin of the
//     posterior on ModelMinFeatures known features;
//   - a view labelled by a generic rule, or by another machine, gets
//     another category when its title alone gives that one
//     ModelOverrideMin, leaving out what that rule (or machine) taught,
//     and ModelMinFeatures of its words are for it.
//
// Manual labels, call apps and call time, meetings, modules, the plan,
// time away and edits are never touched. A project filed under another category is dropped.
func (rs *Ruleset) ApplyModel(m *Model, views []SpanView, labels []Label) {
	if m == nil {
		return
	}
	generic := rs.genericRefs()
	for i := range views {
		v, l := views[i], &labels[i]
		if v.BundleID == span.IdleBundleID || strings.HasPrefix(v.BundleID, "tock.") || (v.ID <= 0 && !IsRemote(v)) {
			continue
		}
		pick := func(v SpanView) string {
			switch src := l.CategorySource; {
			case l.Category == "" && (src == SourceNone || src == ""),
				src == SourceCall && !rs.isCallView(&v): // a call is context: activity first
				c, _ := m.Fill(v)
				return c
			case (src == SourceRule || src == SourceLearned) && generic[l.CategoryRule],
				src == SourceManual && IsRemote(v):
				blind := l.CategoryRule
				if src == SourceManual {
					blind = RemoteBlind(v, l.Category)
				}
				if got, p, _ := m.PredictTitle(v, blind); got != l.Category && p >= ModelOverrideMin && m.backers(v, got, blind) >= ModelMinFeatures {
					return got
				}
			}
			return ""
		}
		c := pick(v)
		if c == "" || !rs.HasCategory(c) {
			continue
		}
		source := SourceModel
		if len(v.Screen) > 0 {
			blindV := v
			blindV.Screen = nil
			if pick(blindV) != c {
				source = SourceScreen // only the screen words decided it
			}
		}
		l.Category, l.Kind, l.CategorySource, l.CategoryRule = c, rs.Kind(c), source, ""
		l.Quiet = rs.quiet(c, "")
		if pc := rs.ProjectCategory(l.Project); l.Project != "" && pc != "" && !strings.EqualFold(pc, c) {
			l.Project, l.ProjectSource, l.ProjectRule = "", SourceNone, ""
		}
	}
}

// Fill is the category for a view no rule labelled, when the model is sure:
// ModelFillMin of the posterior on ModelMinFeatures known features, and
// either the site is known or the title alone agrees, so the app's habits
// never decide on their own.
func (m *Model) Fill(v SpanView) (string, bool) {
	c, p, n := m.Predict(v)
	if c == "" || p < ModelFillMin || n < ModelMinFeatures {
		return "", false
	}
	if h := hostOf(&v); h == "" || m.meta["h:"+h] == nil {
		if t, _, _ := m.PredictTitle(v, ""); t != c {
			return "", false
		}
	}
	return c, true
}

// SeedWords are study words in French and English. They give a new install
// a head start: the category SeedCategory names learns them before you have
// labelled anything.
var SeedWords = []string{
	"théorème", "lemme", "lemma", "corollaire", "démonstration", "proof", "preuve", "exercice", "exercise",
	"cours", "lecture", "leçon", "lesson", "chapitre", "chapter", "examen", "exam", "concours",
	"olympiade", "olympiad", "math", "maths", "mathématiques", "mathematics", "algèbre", "algebra",
	"analyse", "analysis", "calculus", "intégrale", "integral", "dérivée", "derivative", "matrice", "matrix",
	"vecteur", "vector", "polynôme", "polynomial", "équation", "equation", "fonction", "function", "suite",
	"série", "series", "limite", "continuité", "topologie", "topology", "probabilité", "probability",
	"statistique", "statistics", "physique", "physics", "mécanique", "mechanics", "thermodynamique",
	"thermodynamics", "électromagnétisme", "optique", "optics", "quantique", "quantum", "chimie",
	"chemistry", "conjecture", "problème", "problem", "corrigé", "géométrie", "geometry", "arithmétique",
	"arithmetic", "combinatoire", "combinatorics", "inégalité", "inequality", "récurrence", "induction",
	"logique", "dénombrement", "tutorial", "tutoriel", "apprendre", "learn", "course", "révision",
	"homework", "devoir", "énoncé", "solution", "mpsi", "prépa", "licence", "university", "université",
}

var seedName = regexp.MustCompile(`(?i)stud|learn|[ée]tud|cours|school|[ée]cole|r[ée]vis`)

// SeedCategory is the deep category that looks like study (Study, Learning,
// Études, ...), "" when there is none.
func (rs *Ruleset) SeedCategory() string {
	for _, c := range rs.Categories {
		if c.Kind == KindDeep && seedName.MatchString(c.Name) {
			return c.Name
		}
	}
	return ""
}

// Unclear reports whether l says little about what the window holds: no
// category, or one from a rule that names only the app or site. Screen
// reading looks at such windows (OCR-01).
func (rs *Ruleset) Unclear(l Label) bool {
	switch l.CategorySource {
	case SourceNone, "":
		return true
	case SourceRule, SourceLearned:
		return rs.genericRefs()[l.CategoryRule]
	}
	return false
}

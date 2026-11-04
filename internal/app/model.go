package app

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"slices"
	"strings"

	"tock/internal/classify"
	"tock/internal/store"
)

// modelEveryMS is how old the model may get while the labels stay the same:
// new activity is something to learn from too.
const modelEveryMS = 15 * 60_000

// labelModel is the local classifier trained on every labelled view (see
// classify.TrainingSet), plus the study seed words when the rules have a
// study category. It is rebuilt when a label, a rule or the other
// machines' time changes, and every modelEveryMS. nil when [model] is off
// or nothing is labelled yet.
func (a *App) labelModel() (*classify.Model, error) {
	if !a.cfg.Model.Enabled {
		return nil, nil
	}
	now := a.nowMS()
	if a.model != nil && a.modelRS == a.rs && a.modelFP == a.pastKey && now-a.modelAt < modelEveryMS {
		return a.model, nil
	}
	key := a.modelKey()
	if m, at := a.loadModel(key, now); m != nil {
		a.model, a.modelRS, a.modelFP, a.modelAt = m, a.rs, a.pastKey, at
		return m, nil
	}
	ex, err := a.trainingSet()
	if err != nil {
		return nil, err
	}
	var seed map[string][]string
	if c := a.rs.SeedCategory(); c != "" {
		seed = map[string][]string{c: classify.SeedWords}
	}
	if len(ex) == 0 && seed == nil {
		return nil, nil
	}
	a.model, a.modelRS, a.modelFP, a.modelAt = classify.Train(ex, seed), a.rs, a.pastKey, now
	a.saveModel(key, a.model, now)
	return a.model, nil
}

// modelName is the derived row of the trained model.
const modelName = "model"

// modelKey is the key the model is kept under: the inputs every derived
// result depends on and the labels (pastKey). New activity is not in it:
// a model is used for modelEveryMS, as within one process. "" when an
// input cannot be read.
func (a *App) modelKey() string {
	h, ok := a.inputsHash()
	if !ok {
		return ""
	}
	return fmt.Sprintf("%016x-%016x", h, a.pastKey)
}

// modelRow is the model as the derived table keeps it.
type modelRow struct {
	AtMS  int64
	Model []byte
}

// loadModel is the model a process (trackd, usually) trained under key
// less than modelEveryMS ago, and when; nil when there is none.
func (a *App) loadModel(key string, now int64) (*classify.Model, int64) {
	if key == "" {
		return nil, 0
	}
	rows, err := a.st.DerivedRows([]string{modelName})
	if err != nil || rows[modelName].Key != key {
		return nil, 0
	}
	var r modelRow
	if gob.NewDecoder(bytes.NewReader(rows[modelName].Data)).Decode(&r) != nil || r.AtMS > now || now-r.AtMS >= modelEveryMS {
		return nil, 0
	}
	m := new(classify.Model)
	if m.UnmarshalBinary(r.Model) != nil {
		return nil, 0
	}
	return m, r.AtMS
}

// saveModel keeps m for other processes. A cache: a failed write only
// costs the next one a training.
func (a *App) saveModel(key string, m *classify.Model, now int64) {
	if key == "" {
		return
	}
	b, err := m.MarshalBinary()
	if err != nil {
		return
	}
	var buf bytes.Buffer
	if gob.NewEncoder(&buf).Encode(modelRow{AtMS: now, Model: b}) != nil {
		return
	}
	_ = a.st.PutDerived(map[string]store.Derived{modelName: {Key: key, Data: buf.Bytes()}}, now)
}

// ModelStats is how well the local model does on labels it was not trained
// on: every fifth key of the labelled history, by a hash of the key.
type ModelStats struct {
	Examples, Held int // examples trained on, held out
	// Right is held-out examples whose top category is the label; Sure is
	// those that reach classify.ModelFillMin, SureRight the right ones.
	Right, Sure, SureRight int
	// AppRight is held-out examples the app alone gets right: the category
	// with the most training weight for that bundle. What to beat.
	AppRight int
	// Titled is held-out labels that looked at the title (yours, rules with
	// a title, modules): the ones an app's label can't explain. TitledRight
	// and TitledAppRight are the model's and the app's right answers there.
	Titled, TitledRight, TitledAppRight int
	PerCategory                         []ModelRow // by held-out examples, most first
	Confusions                          []ModelConfusion
}

// ModelRow is one category's held-out examples and the right answers.
type ModelRow struct {
	Category    string
	Held, Right int
}

// ModelConfusion is a label the model mistook for another, and how often.
type ModelConfusion struct {
	Label, Guess string
	N            int
}

// ModelStats trains a model without every fifth key and tests it on them.
func (a *App) ModelStats() (ModelStats, error) {
	var st ModelStats
	vs, labels, _, err := a.history()
	if err != nil {
		return st, err
	}
	var train, held []classify.Example
	for _, e := range a.rs.TrainingSet(vs, labels) {
		if fnv32(classify.Key(e.View))%5 == 0 {
			held = append(held, e)
		} else {
			train = append(train, e)
		}
	}
	var seed map[string][]string
	if c := a.rs.SeedCategory(); c != "" {
		seed = map[string][]string{c: classify.SeedWords}
	}
	m := classify.Train(train, seed)
	st.Examples, st.Held = len(train), len(held)
	byApp := map[string]map[string]float64{}
	for _, e := range train {
		if byApp[e.View.BundleID] == nil {
			byApp[e.View.BundleID] = map[string]float64{}
		}
		byApp[e.View.BundleID][e.Category] += e.Weight
	}
	appGuess := func(bundle string) string {
		best, bw := "", 0.0
		for c, w := range byApp[bundle] {
			if w > bw || (w == bw && c < best) {
				best, bw = c, w
			}
		}
		return best
	}
	rows := map[string]*ModelRow{}
	conf := map[[2]string]int{}
	for _, e := range held {
		r := rows[e.Category]
		if r == nil {
			r = &ModelRow{Category: e.Category}
			rows[e.Category] = r
		}
		r.Held++
		appOK := appGuess(e.View.BundleID) == e.Category
		if appOK {
			st.AppRight++
		}
		c, _, _ := m.Predict(e.View)
		got, sure := m.Fill(e.View)
		if e.Blind == "" {
			st.Titled++
			if appOK {
				st.TitledAppRight++
			}
			if c == e.Category {
				st.TitledRight++
			}
		}
		if sure {
			st.Sure++
			if got == e.Category {
				st.SureRight++
			}
		}
		if c == e.Category {
			st.Right++
			r.Right++
			continue
		}
		if c != "" {
			conf[[2]string{e.Category, c}]++
		}
	}
	for _, r := range rows {
		st.PerCategory = append(st.PerCategory, *r)
	}
	slices.SortFunc(st.PerCategory, func(x, y ModelRow) int {
		if x.Held != y.Held {
			return y.Held - x.Held
		}
		return strings.Compare(x.Category, y.Category)
	})
	for k, n := range conf {
		st.Confusions = append(st.Confusions, ModelConfusion{Label: k[0], Guess: k[1], N: n})
	}
	slices.SortFunc(st.Confusions, func(x, y ModelConfusion) int {
		if x.N != y.N {
			return y.N - x.N
		}
		return strings.Compare(x.Label+x.Guess, y.Label+y.Guess)
	})
	return st, nil
}

// ModelExplain ranks the categories for a window of app with title: on
// everything, on the title alone (what can replace a generic rule), and,
// with label, on the title without what label taught on that app (what can
// replace that label; app is "<machine> · <app>" for another machine's).
func (a *App) ModelExplain(app, title, label string) (all, byTitle, without, because []classify.Scored, err error) {
	m, err := a.labelModel()
	if err != nil || m == nil {
		return nil, nil, nil, nil, err
	}
	bundle := app
	if _, b, ok := strings.Cut(app, " · "); ok {
		bundle = b
	}
	v := classify.SpanView{ID: 1, BundleID: bundle, App: app, Title: &title}
	all, _ = m.Rank(v, false, "")
	byTitle, _ = m.Rank(v, true, "")
	blind, top := "", byTitle
	if label != "" {
		blind = classify.RemoteBlind(v, label)
		without, _ = m.Rank(v, true, blind)
		top = without
	}
	if len(top) > 0 {
		because = m.Because(v, top[0].Category, blind)
	}
	return all, byTitle, without, because, nil
}

func fnv32(s string) uint32 {
	h := uint32(2166136261)
	for i := 0; i < len(s); i++ {
		h = (h ^ uint32(s[i])) * 16777619
	}
	return h
}

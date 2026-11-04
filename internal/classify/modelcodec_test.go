package classify

import (
	"math/rand"
	"reflect"
	"testing"

	"tock/internal/span"
)

// A model read back predicts exactly as the one trained.
func TestModelRoundTrip(t *testing.T) {
	ex := []Example{
		{View: tv("zathura", "Thermodynamique MPSI.pdf", 0), Category: "Study", Weight: 1},
		{View: tv("zathura", "Physique MPSI exercices.pdf", 0), Category: "Study", Weight: 1},
		{View: tv("chromium", "Funny cats - YouTube", 0), Category: "Entertainment", Weight: 1, Blind: "rule"},
		{View: tv("wez", "nvim main.go", 0), Category: "Code", Weight: 2},
	}
	for _, m := range []*Model{Train(ex, map[string][]string{"Study": SeedWords}), Train(nil, nil)} {
		b, err := m.MarshalBinary()
		if err != nil {
			t.Fatal(err)
		}
		var back Model
		if err := back.UnmarshalBinary(b); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(&back, m) {
			t.Fatalf("model changed on the way:\n%+v\n%+v", &back, m)
		}
		for _, v := range []SpanView{tv("zathura", "Physique MPSI.pdf", 0), tv("chromium", "cats", 0), tv("x", "", 0)} {
			c1, p1, n1 := m.Predict(v)
			c2, p2, n2 := back.Predict(v)
			if c1 != c2 || p1 != p2 || n1 != n2 {
				t.Fatalf("predict %v: %s %v %d vs %s %v %d", v, c1, p1, n1, c2, p2, n2)
			}
		}
	}
	var bad Model
	if bad.UnmarshalBinary([]byte("junk")) == nil {
		t.Fatal("junk must fail to decode")
	}
}

// The parts of consecutive stretches, merged, are the training set of the
// whole: what lets closed days keep their part.
func TestTrainingPartsMerge(t *testing.T) {
	rs, err := Parse([]byte(precedenceRules), []byte(precedenceLearned))
	if err != nil {
		t.Fatal(err)
	}
	r := rand.New(rand.NewSource(3))
	bundles := []string{"x", "p", "learned", "other", span.IdleBundleID}
	titles := []string{"Algebre", "notes", "learned", ""}
	for n := 0; n < 500; n++ {
		var vs []SpanView
		for i := 0; i < 40; i++ {
			v := SpanView{ID: int64(i + 1), Start: int64(i * 10), End: int64(i*10 + 1 + r.Intn(9)), BundleID: bundles[r.Intn(len(bundles))]}
			v.App = v.BundleID
			if tt := titles[r.Intn(len(titles))]; tt != "" {
				v.Title = &tt
			}
			if r.Intn(6) == 0 {
				v.ID = -(RemoteIDBase + int64(i))
			}
			vs = append(vs, v)
		}
		ovs := map[int64]Override{}
		for i := range vs {
			if r.Intn(5) == 0 {
				ovs[vs[i].ID] = Override{Category: []string{"Fun", "Study", "Rule"}[r.Intn(3)]}
			}
		}
		ls := rs.LabelAll(vs, ovs)
		var parts [][]TrainEntry
		for at := 0; at < len(vs); {
			end := min(len(vs), at+1+r.Intn(12))
			parts = append(parts, rs.TrainingPart(vs[at:end], ls[at:end]))
			at = end
		}
		if got, want := MergeTraining(parts...), rs.TrainingSet(vs, ls); !reflect.DeepEqual(got, want) {
			t.Fatalf("case %d: merged parts differ\n%+v\n%+v", n, got, want)
		}
	}
}

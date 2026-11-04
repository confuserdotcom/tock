package classify

import (
	"reflect"
	"testing"
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

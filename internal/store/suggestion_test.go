package store

import (
	"reflect"
	"testing"
)

func TestSuggestionsRoundTrip(t *testing.T) {
	s := openAt(t, dbPath(t))
	defer s.Close()
	if m, err := s.Suggestions(); err != nil || len(m) != 0 {
		t.Fatalf("empty = %v, %v", m, err)
	}
	in := []Suggestion{
		{Key: "Arc · a.com", Category: "Code", Project: "tock", Model: "m", CreatedMS: 5},
		{Key: "Arc · b.com", Model: "m", CreatedMS: 5}, // no suggestion
	}
	if err := s.SaveSuggestions(in); err != nil {
		t.Fatal(err)
	}
	in[0].Project, in[0].CreatedMS = "", 9
	if err := s.SaveSuggestions(in[:1]); err != nil {
		t.Fatal(err)
	}
	got, err := s.Suggestions()
	want := map[string]Suggestion{in[0].Key: in[0], in[1].Key: in[1]}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("Suggestions = %+v, %v; want %+v", got, err, want)
	}
}

func TestClaim(t *testing.T) {
	s := openAt(t, dbPath(t))
	defer s.Close()
	const hour = 3_600_000
	steps := []struct {
		now, every int64
		want       bool
	}{
		{1_000_000, hour, true},             // first claim
		{1_000_000 + hour - 1, hour, false}, // within the hour
		{1_000_000 + hour, hour, true},      // hour over
		{1_000_000 + hour + 5, 0, true},     // force
		{1_000_000 + hour + 4, 0, false},    // clock went back
	}
	for i, st := range steps {
		if ok, err := s.Claim(KeyLLMClaim, st.now, st.every); err != nil || ok != st.want {
			t.Fatalf("step %d: Claim = %v, %v; want %v", i, ok, err, st.want)
		}
	}
	if v, ok, err := s.State(KeyLLMClaim); err != nil || !ok || v != "4600005" {
		t.Fatalf("state = %q %v %v", v, ok, err)
	}
}

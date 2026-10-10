package suggest

import (
	"encoding/json"
	"reflect"
	"testing"

	"tock/internal/classify"
)

func TestParse(t *testing.T) {
	b := Batch{Keys: map[string]string{"k1": "A", "k2": "B", "k3": "C"}}
	raw := json.RawMessage(`{"suggestions":[
		{"id":"k2","category":"code","project":"TOCK"},
		{"id":"k9","category":"Code","project":null},
		{"id":"k1","category":"Nope","project":"Old"}]}`)
	got, err := Parse(raw, b, ruleset(t))
	want := []Suggestion{{Key: "A"}, {Key: "B", Category: "Code", Project: "tock"}, {Key: "C"}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("Parse = %+v, %v; want %+v", got, err, want)
	}
}

func TestParseBadInput(t *testing.T) {
	b := Batch{Keys: map[string]string{"k1": "A"}}
	if _, err := Parse(json.RawMessage(`[1,2]`), b, ruleset(t)); err == nil {
		t.Fatal("want an error for a non-object tool input")
	}
}

func TestCanon(t *testing.T) {
	rs := ruleset(t)
	tests := []struct {
		cat, proj string
		want      classify.Want
		ok        bool
	}{
		{"code", "", classify.Want{Category: "Code"}, true},
		{"", "TOCK", classify.Want{Project: "tock"}, true},
		{"Video", "tock", classify.Want{Category: "Video", Project: "tock"}, true},
		{"Gone", "tock", classify.Want{}, false},
		{"Code", "Old", classify.Want{}, false},
		{"", "", classify.Want{}, false},
	}
	for _, tt := range tests {
		if w, ok := Canon(rs, tt.cat, tt.proj); w != tt.want || ok != tt.ok {
			t.Errorf("Canon(%q, %q) = %+v %v, want %+v %v", tt.cat, tt.proj, w, ok, tt.want, tt.ok)
		}
	}
}

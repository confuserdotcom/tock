package suggest

import (
	"fmt"
	"strings"
	"testing"

	"tock/internal/classify"
	"tock/internal/config"
)

// LLM-03 / success criterion 2: private and denylisted items are never
// serialised, and neither is any raw example title.
func TestPayloadNeverCarriesDeniedItems(t *testing.T) {
	c := config.DefaultLLM()
	c.DenyHosts = append(c.DenyHosts, "secret-host")
	c.DenyTitles = []string{"payroll"}
	items := []classify.Item{
		{Key: "Code · main.go", App: "Code", TotalMS: 10 * minute, Bundles: []string{"com.microsoft.VSCode"}, Example: "RAWEXAMPLE main.go"},
		{Key: "MarkerPrivApp (private)", App: "MarkerPrivApp", TotalMS: 9 * minute, Private: true, Bundles: []string{"arc"}},
		{Key: "Mail · markermail subject", App: "Mail", TotalMS: 8 * minute, Bundles: []string{"com.apple.mail"}, Example: "RAWEXAMPLE mail"},
		{Key: "Arc · mabanque.bnpparibas.fr/markerbank", App: "Arc", TotalMS: 7 * minute, ByURL: true, Bundles: []string{"arc"}},
		{Key: "Arc · secret-host.example/markerhost", App: "Arc", TotalMS: 6 * minute, ByURL: true, Bundles: []string{"arc"}},
		{Key: "Notes · markerpayroll Payroll q#", App: "Notes", TotalMS: 5 * minute, Bundles: []string{"notes"}},
		{Key: "Notes · markercached", App: "Notes", TotalMS: 4 * minute, Bundles: []string{"notes"}},
		{Key: "Notes · markershort", App: "Notes", TotalMS: minute, Bundles: []string{"notes"}},
	}
	b := Prepare(items, map[string]bool{"Notes · markercached": true}, filter(t, c), ruleset(t))
	s := string(b.Payload)
	for _, m := range []string{"MarkerPrivApp", "markermail", "markerbank", "markerhost", "markerpayroll", "markercached", "markershort", "RAWEXAMPLE"} {
		if strings.Contains(s, m) {
			t.Errorf("payload carries %q:\n%s", m, s)
		}
	}
	if len(b.Keys) != 1 || b.Keys["k1"] != "Code · main.go" || !strings.Contains(s, `"title": "main.go"`) {
		t.Fatalf("keys = %v payload:\n%s", b.Keys, s)
	}
}

func TestPayloadShape(t *testing.T) {
	items := []classify.Item{
		{Key: "Arc · github.com/tock", App: "Arc", TotalMS: 150_000, ByURL: true, Bundles: []string{"arc"}},
		{Key: "Finder", App: "Finder", TotalMS: 120_000, Bundles: []string{"finder"}},
	}
	b := Prepare(items, nil, filter(t, config.DefaultLLM()), ruleset(t))
	want := `{
  "categories": [
    {
      "name": "Code",
      "kind": "deep"
    },
    {
      "name": "Video",
      "kind": "distract"
    }
  ],
  "projects": [
    "tock"
  ],
  "items": [
    {
      "id": "k1",
      "app": "Arc",
      "host": "github.com/tock",
      "minutes": 3
    },
    {
      "id": "k2",
      "app": "Finder",
      "minutes": 2
    }
  ]
}`
	if string(b.Payload) != want {
		t.Fatalf("payload:\n%s\nwant:\n%s", b.Payload, want)
	}
	if b.Keys["k1"] != "Arc · github.com/tock" || b.Keys["k2"] != "Finder" {
		t.Fatalf("keys = %v", b.Keys)
	}
}

func TestPrepareCapsAndKeepsOrder(t *testing.T) {
	var items []classify.Item
	for i := range 45 {
		items = append(items, classify.Item{Key: fmt.Sprintf("App · t%02d", i), App: "App",
			TotalMS: int64(100-i) * minute, Bundles: []string{"app"}})
	}
	b := Prepare(items, nil, filter(t, config.DefaultLLM()), ruleset(t))
	if len(b.Keys) != BatchMax || b.Keys["k1"] != "App · t00" || b.Keys["k40"] != "App · t39" {
		t.Fatalf("keys = %d, k1 %q, k40 %q", len(b.Keys), b.Keys["k1"], b.Keys["k40"])
	}
}

func TestPrepareNothing(t *testing.T) {
	b := Prepare(nil, nil, filter(t, config.DefaultLLM()), ruleset(t))
	if len(b.Keys) != 0 || b.Payload != nil {
		t.Fatalf("empty batch = %+v", b)
	}
}

func TestFilterRules(t *testing.T) {
	c := config.DefaultLLM()
	c.DenyTitles = []string{"salary"}
	f := filter(t, c)
	tests := []struct {
		it   classify.Item
		want bool
	}{
		{classify.Item{Key: "A · x", App: "A", TotalMS: 2 * minute, Bundles: []string{"a"}}, true},
		{classify.Item{Key: "A · x", App: "A", TotalMS: 2*minute - 1, Bundles: []string{"a"}}, false},
		{classify.Item{Key: "A (private)", App: "A", TotalMS: 9 * minute, Private: true}, false},
		{classify.Item{Key: "A · x", App: "A", TotalMS: 9 * minute, Bundles: []string{"a", "com.bitwarden.desktop"}}, false},
		{classify.Item{Key: "Arc · www.paypal.com/x", App: "Arc", TotalMS: 9 * minute, ByURL: true, Bundles: []string{"arc"}}, false},
		{classify.Item{Key: "Arc · mail.google.com/mail", App: "Arc", TotalMS: 9 * minute, ByURL: true, Bundles: []string{"arc"}}, false},
		{classify.Item{Key: "Arc · wiseguys.dev/x", App: "Arc", TotalMS: 9 * minute, ByURL: true, Bundles: []string{"arc"}}, true},
		{classify.Item{Key: "Numbers · Salary #", App: "Numbers", TotalMS: 9 * minute, Bundles: []string{"n"}}, false},
	}
	for _, tt := range tests {
		if got := f.Allow(tt.it); got != tt.want {
			t.Errorf("Allow(%q) = %v, want %v", tt.it.Key, got, tt.want)
		}
	}
}

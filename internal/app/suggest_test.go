package app

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"tock/internal/apptest"
	"tock/internal/classify"
	"tock/internal/suggest"
)

// apptest.DaySpans inbox, largest first: Arc · chatgpt.com/c (12m),
// Mail · inbox (10m), Arc · labs.x.tv/lab (6m).
func TestSuggestPrepareSaveAndCache(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	a := open(t, e)

	p, err := a.SuggestPrepare(false)
	if err != nil || p.Status != suggest.Ready || p.Model != "claude-haiku-5-5" || len(p.Batch.Keys) != 3 ||
		p.Batch.Keys["k1"] != "Arc · chatgpt.com/c" || p.Batch.Keys["k3"] != "Arc · labs.x.tv/lab" {
		t.Fatalf("first prepare = %+v, %v", p, err)
	}
	if !strings.Contains(string(p.Batch.Payload), `"host": "chatgpt.com/c"`) || strings.Contains(string(p.Batch.Payload), "Proofs") {
		t.Fatalf("payload:\n%s", p.Batch.Payload)
	}
	if again, err := a.SuggestPrepare(false); err != nil || again.Status != suggest.Wait ||
		again.NextMS != e.Clock.UnixMilli()+suggest.EveryMS {
		t.Fatalf("second prepare = %+v, %v; want Wait until +1h", again, err)
	}

	raw := json.RawMessage(`{"suggestions":[{"id":"k1","category":"study","project":"TOCK"},{"id":"k2","category":"Nope","project":null}]}`)
	if n, err := a.SuggestSave(p.Batch, p.Model, raw); err != nil || n != 1 {
		t.Fatalf("save = %d, %v", n, err)
	}
	got, err := a.Suggestions()
	want := map[string]classify.Want{"Arc · chatgpt.com/c": {Category: "Study", Project: "tock"}}
	if err != nil || len(got) != 1 || got["Arc · chatgpt.com/c"] != want["Arc · chatgpt.com/c"] {
		t.Fatalf("Suggestions = %+v, %v", got, err)
	}

	// Every key of the batch is cached, "none" included: nothing left to send.
	e.Clock = e.Clock.Add(time.Hour)
	if p, err := a.SuggestPrepare(false); err != nil || p.Status != suggest.Nothing {
		t.Fatalf("after save = %+v, %v; want Nothing", p, err)
	}
	if b, err := a.SuggestBatch(); err != nil || len(b.Keys) != 0 {
		t.Fatalf("dry batch = %+v, %v", b, err)
	}
}

func TestSuggestForceAndOff(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	a := open(t, e)
	if p, _ := a.SuggestPrepare(false); p.Status != suggest.Ready {
		t.Fatalf("first = %+v", p)
	}
	if p, err := a.SuggestPrepare(true); err != nil || p.Status != suggest.Ready {
		t.Fatalf("force = %+v, %v; want Ready (nothing cached yet)", p, err)
	}
	e.WriteConfig(t, "[llm]\nenabled = false\n")
	if _, err := a.Reload(); err != nil {
		t.Fatal(err)
	}
	if p, err := a.SuggestPrepare(true); err != nil || p.Status != suggest.Off {
		t.Fatalf("disabled = %+v, %v; want Off", p, err)
	}
}

func TestSuggestionsHiddenWhenNamesGo(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	a := open(t, e)
	p, _ := a.SuggestPrepare(false)
	raw := json.RawMessage(`{"suggestions":[{"id":"k1","category":"Study","project":null}]}`)
	if _, err := a.SuggestSave(p.Batch, p.Model, raw); err != nil {
		t.Fatal(err)
	}
	e.WriteRules(t, strings.Replace(apptest.Rules, "[[category]]\nname = \"Study\"\nkind = \"deep\"\n\n", "", 1))
	if _, err := a.Reload(); err != nil {
		t.Fatal(err)
	}
	if got, err := a.Suggestions(); err != nil || len(got) != 0 {
		t.Fatalf("Suggestions after Study removed = %+v, %v", got, err)
	}
}

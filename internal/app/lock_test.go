package app

import (
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"tock/internal/apptest"
)

// TestRulesWritesDoNotLoseEachOther: two sessions (tock and trackd) that
// rewrite the rule files at the same time both keep their change.
func TestRulesWritesDoNotLoseEachOther(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	a, b := open(t, e), open(t, e)
	var wg sync.WaitGroup
	add := func(x *App, name string) {
		defer wg.Done()
		err := x.rewriteRules(func(rules, learned []byte) ([]byte, []byte, error) {
			time.Sleep(50 * time.Millisecond) // the other writer reads meanwhile
			return append(rules, []byte("\n[[category]]\nname = \""+name+"\"\nkind = \"neutral\"\n")...), learned, nil
		})
		if err != nil {
			t.Error(err)
		}
	}
	wg.Add(2)
	go add(a, "One")
	go add(b, "Two")
	wg.Wait()
	got, _ := os.ReadFile(e.Rules)
	if !strings.Contains(string(got), `"One"`) || !strings.Contains(string(got), `"Two"`) {
		t.Fatalf("a change was lost:\n%s", got)
	}
}

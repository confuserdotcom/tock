package app

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"tock/internal/apptest"
	"tock/internal/classify"
	"tock/internal/store"
)

func TestSnoozeDistract(t *testing.T) {
	e := distracted(t)
	a := open(t, e)
	e.Clock = apptest.Now
	if _, err := a.SnoozeDistract(0, 10*time.Minute); !errors.Is(err, store.ErrNoSession) {
		t.Fatalf("no session: %v, want ErrNoSession", err)
	}
	if v, err := a.distractSnooze(); err != nil || v != 0 {
		t.Fatalf("no session wrote %d, %v", v, err)
	}

	s := startAt(t, e, a, "14:50", 25)
	e.Clock = apptest.Now
	if _, err := a.SnoozeDistract(s.ID+1, 10*time.Minute); !errors.Is(err, store.ErrNoSession) {
		t.Fatalf("other session: %v, want ErrNoSession", err)
	}
	until, err := a.SnoozeDistract(s.ID, 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if want := apptest.Now.Add(10 * time.Minute); !until.Equal(want) || until.Location() != apptest.Zone {
		t.Fatalf("until = %v (%v), want %v in %v", until, until.Location(), want, apptest.Zone)
	}
	if was, err := a.ResumeDistract(); err != nil || !was {
		t.Fatalf("first resume = %v, %v; want true", was, err)
	}
	if was, err := a.ResumeDistract(); err != nil || was {
		t.Fatalf("second resume = %v, %v; want false", was, err)
	}
	// Session 0 means whatever runs.
	if _, err := a.SnoozeDistract(0, time.Minute); err != nil {
		t.Fatal(err)
	}
}

func TestDistractSnoozeSurvivesTick(t *testing.T) {
	e := distracted(t)
	a := open(t, e)
	s := startAt(t, e, a, "14:50", 25)
	e.Clock = apptest.Now
	until, err := a.SnoozeDistract(s.ID, 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	trackd := open(t, e) // a second process on the same files
	for _, hhmm := range []string{"15:00", "15:03"} {
		e.Clock = time.UnixMilli(apptest.At(t, hhmm, 0)).In(apptest.Zone)
		if notes, err := trackd.FocusTick(false); err != nil || len(notes) != 0 {
			t.Fatalf("%s: snoozed tick = %+v, %v", hhmm, notes, err)
		}
	}
	got, err := a.distractSnooze()
	if err != nil || got != until.UnixMilli() {
		t.Fatalf("snooze after ticks = %d, %v; want %d", got, err, until.UnixMilli())
	}
}

func TestFocusCarriesSnooze(t *testing.T) {
	e := distracted(t)
	a := open(t, e)
	e.Clock = apptest.Now
	f, err := a.Focus(e.Clock)
	if err != nil || f.SnoozeMin != 10 || f.SnoozeUntilMS != 0 {
		t.Fatalf("no session: SnoozeMin %d, SnoozeUntilMS %d, %v; want 10, 0", f.SnoozeMin, f.SnoozeUntilMS, err)
	}
	// A snooze left behind with no session running is not reported.
	if err := a.st.SetState(store.KeyFocusDistractSnooze, "99999999999999", 1); err != nil {
		t.Fatal(err)
	}
	if f, err = a.Focus(e.Clock); err != nil || f.SnoozeUntilMS != 0 {
		t.Fatalf("no session, stale key: SnoozeUntilMS %d, %v; want 0", f.SnoozeUntilMS, err)
	}

	s := startAt(t, e, a, "14:50", 25)
	e.Clock = apptest.Now
	if f, err = a.Focus(e.Clock); err != nil || f.SnoozeUntilMS != 0 {
		t.Fatalf("not snoozed: SnoozeUntilMS %d, %v; want 0", f.SnoozeUntilMS, err)
	}
	until, err := a.SnoozeDistract(s.ID, 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if f, err = a.Focus(e.Clock); err != nil || f.SnoozeUntilMS != until.UnixMilli() || f.SnoozeMin != 10 {
		t.Fatalf("snoozed: SnoozeUntilMS %d, SnoozeMin %d, %v; want %d, 10", f.SnoozeUntilMS, f.SnoozeMin, err, until.UnixMilli())
	}
	e.Clock = until.Add(time.Minute)
	if f, err = a.Focus(e.Clock); err != nil || f.SnoozeUntilMS != 0 {
		t.Fatalf("expired: SnoozeUntilMS %d, %v; want 0", f.SnoozeUntilMS, err)
	}
}

// catQuiet reports a category marked interrupt = false.
func catQuiet(a *App, name string) bool {
	for _, c := range a.Rules().Categories {
		if strings.EqualFold(c.Name, name) {
			return c.Quiet
		}
	}
	return false
}

func slurp(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	return string(b)
}

func TestSetCategoryInterrupt(t *testing.T) {
	e := distracted(t)
	a := open(t, e)
	if err := a.SetCategoryInterrupt("entertainment", false); err != nil {
		t.Fatal(err)
	}
	if !catQuiet(a, "Entertainment") {
		t.Fatalf("Entertainment not quiet: %+v", a.Rules().Categories)
	}
	text := slurp(t, e.Rules)
	if !strings.Contains(text, "name = \"Entertainment\"\ninterrupt = false\nkind = \"distract\"\n") {
		t.Fatalf("rules.toml:\n%s", text)
	}
	if slurp(t, e.Rules+".bak") != apptest.Rules {
		t.Fatal("rules.toml.bak is not the previous rules.toml")
	}
	if err := a.SetCategoryInterrupt("Entertainment", true); err != nil {
		t.Fatal(err)
	}
	if catQuiet(a, "Entertainment") || slurp(t, e.Rules) != apptest.Rules {
		t.Fatalf("unmark left %q", slurp(t, e.Rules))
	}
	// A notice names the category exactly: no prefix match.
	before := slurp(t, e.Rules)
	if err := a.SetCategoryInterrupt("Ent", false); !errors.Is(err, ErrCategoryGone) {
		t.Fatalf("Ent: %v, want ErrCategoryGone", err)
	}
	if slurp(t, e.Rules) != before {
		t.Fatal("a refused mark changed rules.toml")
	}
}

func TestMarkRuleQuiet(t *testing.T) {
	e := distracted(t)
	a := open(t, e)
	r, ok := a.Rules().RuleByRef("rules.toml#3")
	if !ok {
		t.Fatal("no rules.toml#3")
	}
	if err := a.MarkRuleQuiet("rules.toml#3", r.Fingerprint(), "entertainment"); err != nil {
		t.Fatal(err)
	}
	if r, _ := a.Rules().RuleByRef("rules.toml#3"); !r.Quiet {
		t.Fatalf("rule #3 not quiet: %+v", r)
	}
	text := slurp(t, e.Rules)
	if strings.Count(text, "interrupt = false") != 1 || !strings.Contains(text, "[[rule]]\ninterrupt = false\nurl = ") {
		t.Fatalf("rules.toml:\n%s", text)
	}
	if catQuiet(a, "Entertainment") {
		t.Fatal("marking a rule marked the whole category")
	}
	if slurp(t, e.Rules+".bak") != apptest.Rules {
		t.Fatal("rules.toml.bak is not the previous rules.toml")
	}
}

func TestMarkRuleQuietStale(t *testing.T) {
	t.Run("rule inserted before", func(t *testing.T) {
		e := distracted(t)
		a := open(t, e)
		r, _ := a.Rules().RuleByRef("rules.toml#3")
		moved := strings.Replace(apptest.Rules, "[[rule]]\nbundle = [\"term\"]\ntitle", "[[rule]]\napp = \"Slack\"\ncategory = \"Communication\"\n\n[[rule]]\nbundle = [\"term\"]\ntitle", 1)
		e.WriteRules(t, moved)
		if _, err := a.Reload(); err != nil {
			t.Fatal(err)
		}
		if err := a.MarkRuleQuiet("rules.toml#3", r.Fingerprint(), "Entertainment"); !errors.Is(err, ErrRuleChanged) {
			t.Fatalf("err = %v, want ErrRuleChanged", err)
		}
		if slurp(t, e.Rules) != moved {
			t.Fatal("rules.toml changed")
		}
	})
	t.Run("learned rule renumbered", func(t *testing.T) {
		e := distracted(t)
		if err := classify.AppendLearned(e.Learned, classify.Rule{URL: "reddit", Category: "Entertainment"}, "first"); err != nil {
			t.Fatal(err)
		}
		a := open(t, e)
		r, ok := a.Rules().RuleByRef("learned.toml#1")
		if !ok {
			t.Fatal("no learned.toml#1")
		}
		if err := classify.AppendLearned(e.Learned, classify.Rule{URL: "twitch", Category: "Entertainment"}, "second"); err != nil {
			t.Fatal(err)
		}
		if _, err := a.Reload(); err != nil {
			t.Fatal(err)
		}
		rules, learned := slurp(t, e.Rules), slurp(t, e.Learned)
		if err := a.MarkRuleQuiet("learned.toml#1", r.Fingerprint(), "Entertainment"); !errors.Is(err, ErrRuleChanged) {
			t.Fatalf("err = %v, want ErrRuleChanged", err)
		}
		if slurp(t, e.Rules) != rules || slurp(t, e.Learned) != learned {
			t.Fatal("a refused mark changed a file")
		}
		// The same ref, fingerprinted now, marks the learned file only.
		cur, _ := a.Rules().RuleByRef("learned.toml#2")
		if err := a.MarkRuleQuiet("learned.toml#2", cur.Fingerprint(), "Entertainment"); err != nil {
			t.Fatal(err)
		}
		if got := slurp(t, e.Learned); strings.Count(got, "interrupt = false") != 1 || slurp(t, e.Rules) != rules {
			t.Fatalf("learned.toml:\n%s", got)
		}
	})
	t.Run("recategorised", func(t *testing.T) {
		e := distracted(t)
		a := open(t, e)
		r, _ := a.Rules().RuleByRef("rules.toml#3")
		changed := strings.Replace(apptest.Rules, "category = \"Entertainment\"\n", "category = \"Communication\"\n", 1)
		e.WriteRules(t, changed)
		if _, err := a.Reload(); err != nil {
			t.Fatal(err)
		}
		if err := a.MarkRuleQuiet("rules.toml#3", r.Fingerprint(), "Entertainment"); !errors.Is(err, ErrRuleChanged) {
			t.Fatalf("err = %v, want ErrRuleChanged", err)
		}
		if slurp(t, e.Rules) != changed {
			t.Fatal("rules.toml changed")
		}
	})
	t.Run("gone", func(t *testing.T) {
		e := distracted(t)
		a := open(t, e)
		if err := a.MarkRuleQuiet("rules.toml#9", "x", "Entertainment"); !errors.Is(err, ErrRuleChanged) {
			t.Fatalf("err = %v, want ErrRuleChanged", err)
		}
	})
}

func TestMarkWritesNoUndo(t *testing.T) {
	e := distracted(t)
	a := open(t, e)
	if err := a.Tag([]int64{1}, classify.Want{Category: "Study"}); err != nil {
		t.Fatal(err)
	}
	before, err := a.UndoList(10)
	if err != nil || len(before) == 0 {
		t.Fatalf("undo list = %+v, %v", before, err)
	}
	r, _ := a.Rules().RuleByRef("rules.toml#3")
	if err := a.SetCategoryInterrupt("Entertainment", false); err != nil {
		t.Fatal(err)
	}
	if err := a.MarkRuleQuiet("rules.toml#3", r.Fingerprint(), "Entertainment"); err != nil {
		t.Fatal(err)
	}
	after, err := a.UndoList(10)
	if err != nil || !reflect.DeepEqual(after, before) {
		t.Fatalf("undo list changed: %+v, want %+v (%v)", after, before, err)
	}
}

func TestEditCategoryInterrupt(t *testing.T) {
	e := distracted(t)
	a := open(t, e)
	on, off := true, false
	if err := a.EditCategory(CategorySpec{Name: "Ent", Interrupt: &off}); err != nil {
		t.Fatal(err)
	}
	if !catQuiet(a, "Entertainment") {
		t.Fatal("Interrupt false did not mark")
	}
	if err := a.EditCategory(CategorySpec{Name: "Entertainment", Kind: classify.KindBrowse}); err != nil {
		t.Fatal(err)
	}
	if !catQuiet(a, "Entertainment") || a.Rules().Kind("Entertainment") != classify.KindBrowse {
		t.Fatal("a nil Interrupt must leave the mark as it is")
	}
	if err := a.EditCategory(CategorySpec{Name: "Entertainment", Interrupt: &on}); err != nil {
		t.Fatal(err)
	}
	if catQuiet(a, "Entertainment") || strings.Contains(slurp(t, e.Rules), "interrupt") {
		t.Fatalf("Interrupt true did not unmark:\n%s", slurp(t, e.Rules))
	}
}

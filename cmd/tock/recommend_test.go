package main

import (
	"strings"
	"testing"
	"time"

	"tock/internal/store"
)

// addPendingRec writes a focus recommendation of 25 min sent ago before
// testNow and returns its id.
func addPendingRec(t *testing.T, te *tenv, ago time.Duration) int64 {
	t.Helper()
	st, err := store.Open(te.dbPath())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	id, err := st.AddRecommend(store.Recommend{Kind: "focus", AtMS: te.now.Add(-ago).UnixMilli(), Weekday: int(te.now.Weekday()), Hour: te.now.Hour(), LengthMS: 25 * 60_000})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func recReply(t *testing.T, te *tenv, id int64) string {
	t.Helper()
	st, err := store.Open(te.dbPath())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	r, err := st.Recommend(id)
	if err != nil {
		t.Fatal(err)
	}
	return r.Reply
}

func TestRecommendCLI(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)

	if code, out := te.run(t, "", "recommend"); code != 0 || out != "no pending recommendation\n" {
		t.Fatalf("none: %d %q", code, out)
	}
	for _, sub := range []string{"accept", "dismiss"} {
		if code, out := te.run(t, "", "recommend", sub); code != 1 || out != "" || te.errb.String() != "tock: no pending recommendation\n" {
			t.Fatalf("%s with none: %d %q %q", sub, code, out, te.errb.String())
		}
	}

	id := addPendingRec(t, te, 10*time.Minute)
	if code, out := te.run(t, "", "recommend"); code != 0 || out != "Suggested: focus now (25 min) · open until 15:20\n" {
		t.Fatalf("show: %d %q", code, out)
	}
	if code, out := te.run(t, "", "recommend", "later"); code != 2 {
		t.Fatalf("unknown arg: %d %q, want exit 2", code, out)
	}
	if code, _ := te.run(t, "", "recommend", "accept", "now"); code != 2 {
		t.Fatalf("two args: %d, want exit 2", code)
	}
	if code, out := te.run(t, "", "recommend", "accept"); code != 0 || out != "started 25 min session\n" {
		t.Fatalf("accept: %d %q %q", code, out, te.errb.String())
	}
	if recReply(t, te, id) != "accept" {
		t.Fatalf("reply = %q, want accept", recReply(t, te, id))
	}
	if code, out := te.run(t, "", "session"); code != 0 || !strings.HasPrefix(out, "session ") || out == "no session\n" {
		t.Fatalf("session after accept: %d %q", code, out)
	}
	if code, _ := te.run(t, "", "recommend", "accept"); code != 1 || te.errb.String() != "tock: no pending recommendation\n" {
		t.Fatalf("second accept: %d %q", code, te.errb.String())
	}
	if code, out := te.run(t, "", "recommend"); code != 0 || out != "no pending recommendation\n" {
		t.Fatalf("show after accept: %d %q", code, out)
	}

	// A session still runs: accept fails and leaves the row open.
	id2 := addPendingRec(t, te, 5*time.Minute)
	if code, out := te.run(t, "", "recommend", "accept"); code != 1 || out != "" || te.errb.String() != "tock: a session is already running\n" {
		t.Fatalf("accept with a session: %d %q %q", code, out, te.errb.String())
	}
	if recReply(t, te, id2) != "" {
		t.Fatalf("reply after a failed accept = %q, want open", recReply(t, te, id2))
	}
	if code, out := te.run(t, "", "recommend", "dismiss"); code != 0 || out != "dismissed: focus now (25 min)\n" {
		t.Fatalf("dismiss: %d %q", code, out)
	}
	if recReply(t, te, id2) != "dismiss" {
		t.Fatalf("reply = %q, want dismiss", recReply(t, te, id2))
	}
}

func TestRecommendCLIBreak(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	st, err := store.Open(te.dbPath())
	if err != nil {
		t.Fatal(err)
	}
	id, err := st.AddRecommend(store.Recommend{Kind: "break", AtMS: te.now.Add(-5 * time.Minute).UnixMilli(), Weekday: int(te.now.Weekday()), Hour: te.now.Hour(), LengthMS: 10 * 60_000})
	st.Close()
	if err != nil {
		t.Fatal(err)
	}
	if code, out := te.run(t, "", "recommend"); code != 0 || out != "Suggested: break (10 min) · open until 15:25\n" {
		t.Fatalf("show: %d %q", code, out)
	}
	want := "break until " + te.now.Add(10*time.Minute).Format("15:04") + "\n"
	if code, out := te.run(t, "", "recommend", "accept"); code != 0 || out != want {
		t.Fatalf("accept: %d %q %q, want %q", code, out, te.errb.String(), want)
	}
	if recReply(t, te, id) != "accept" {
		t.Fatalf("reply = %q, want accept", recReply(t, te, id))
	}
	if code, out := te.run(t, "", "session"); code != 0 || out != "no session\n" {
		t.Fatalf("session after a break: %d %q", code, out)
	}
}

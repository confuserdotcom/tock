package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"tock/internal/focus"
	"tock/internal/store"
)

func heartbeat(t *testing.T, te *tenv) {
	t.Helper()
	st, err := store.Open(te.dbPath())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	hb := te.now.UnixMilli()
	if err := st.SetState(store.KeyHeartbeat, strconv.FormatInt(hb, 10), hb); err != nil {
		t.Fatal(err)
	}
}

func TestSessionCommands(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	if code, out := te.run(t, "", "session"); code != 0 || out != "no session\n" {
		t.Fatalf("show none: %d %q", code, out)
	}
	code, out := te.run(t, "", "session", "start")
	if code != 0 || out != "session started: 25 min, ends 15:25\n" || te.errb.String() != "warning: trackd not running, no alerts\n" {
		t.Fatalf("start: %d %q %q", code, out, te.errb.String())
	}
	if code, _ := te.run(t, "", "session", "start", "50"); code != 1 || te.errb.String() != "tock: session already running\n" {
		t.Fatalf("double start: %d %q", code, te.errb.String())
	}
	te.now = testNow.Add(5 * time.Minute)
	if code, out := te.run(t, "", "session"); code != 0 || out != "session 20:00 left of 25m · focus 50% · deep 0% · 0 interruptions\n" {
		t.Fatalf("show: %d %q", code, out)
	}
	te.now = testNow.Add(10 * time.Minute)
	if code, out := te.run(t, "", "session", "stop"); code != 0 || out != "stopped: 15:00  manual   10m  deep   0%  0 int\n" {
		t.Fatalf("stop: %d %q", code, out)
	}
	if code, _ := te.run(t, "", "session", "stop"); code != 1 || te.errb.String() != "tock: no session\n" {
		t.Fatalf("stop none: %d %q", code, te.errb.String())
	}
}

func TestSessionStartLengthWithTrackdUp(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	heartbeat(t, te)
	if code, out := te.run(t, "", "session", "start", "50"); code != 0 || out != "session started: 50 min, ends 15:50\n" || te.errb.String() != "" {
		t.Fatalf("start 50: %d %q %q", code, out, te.errb.String())
	}
	if code, out := te.run(t, "", "status"); code != 0 || out != "2h54 · score 74 · focus 2h14 · now unlabelled · ◷ 50m\n" {
		t.Fatalf("status: %d %q", code, out)
	}
}

func TestSessionUsage(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	for _, args := range [][]string{
		{"session", "start", "0"}, {"session", "start", "241"}, {"session", "start", "abc"},
		{"session", "start", "1", "2"}, {"session", "stop", "now"}, {"session", "pause"},
		{"recap", "yesterday"}, {"recap", "2026-10-09", "x"},
	} {
		if code, _ := te.run(t, "", args...); code != 2 {
			t.Errorf("%q: code %d, want 2", args, code)
		}
	}
}

func TestRecapCommand(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	if code, out := te.run(t, "", "recap"); code != 0 || out != "2h14 focus, score 74 (target 4h00). Top project: tock.\n" {
		t.Fatalf("recap: %d %q", code, out)
	}
	if code, out := te.run(t, "", "recap", "2026-10-09"); code != 0 || out != "No activity.\n" {
		t.Fatalf("recap yesterday: %d %q", code, out)
	}
}

// addSession stores a finished manual (or auto) session starting at hh:mm
// daysAgo days before testNow and returns it.
func addSession(t *testing.T, te *tenv, hhmm string, daysAgo, n, deepMin, interruptions int, kind string) store.Session {
	t.Helper()
	st, err := store.Open(te.dbPath())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	start := at(t, hhmm) - int64(daysAgo)*24*3600_000
	end := start + int64(n)*60_000
	x := store.Session{Kind: kind, StartMS: start, EndMS: end, DeepMS: int64(deepMin) * 60_000,
		LongestMS: int64(deepMin) * 30_000, Interruptions: interruptions, Switches: 2}
	if kind == store.SessionAuto {
		x.Interruptions = 0
		if ok, err := st.AddAutoSession(x); err != nil || !ok {
			t.Fatalf("AddAutoSession = %v, %v", ok, err)
		}
		got, err := st.Sessions(start, start+1)
		if err != nil || len(got) != 1 {
			t.Fatal(got, err)
		}
		return got[0]
	}
	run, err := st.StartSession(start, end)
	if err != nil {
		t.Fatal(err)
	}
	x.ID = run.ID
	x.PlannedMS = end
	if ok, err := st.EndSession(x); err != nil || !ok {
		t.Fatalf("EndSession = %v, %v", ok, err)
	}
	return x
}

func sessionRow(t *testing.T, te *tenv, id int64) store.Session {
	t.Helper()
	st, err := store.Open(te.dbPath())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s, err := st.SessionByID(id)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func qOf(s store.Session) int {
	return focus.Quality(focus.Session(s), focus.DefaultQualityWeights()).Score
}

func whenOf(s store.Session) string {
	return time.UnixMilli(s.StartMS).In(testZone).Format("Mon 15:04")
}

func TestSessionRate(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	old := addSession(t, te, "08:00", 0, 25, 10, 1, store.SessionManual)
	last := addSession(t, te, "10:00", 0, 50, 40, 1, store.SessionManual)

	code, out := te.run(t, "", "session", "rate", "4", "good", "run")
	want := fmt.Sprintf("rated session %d (%s, 50m, q%d) ★4 · note saved\n", last.ID, whenOf(last), qOf(last))
	if code != 0 || out != want {
		t.Fatalf("rate last: %d %q, want %q", code, out, want)
	}
	if r := sessionRow(t, te, last.ID); r.Rating != 4 || r.Note != "good run" || r.RatedMS != te.now.UnixMilli() {
		t.Fatalf("stored = %+v", r)
	}
	// by id, no note: the stored note is kept
	code, out = te.run(t, "", "session", "rate", "#"+strconv.FormatInt(last.ID, 10), "5")
	want = fmt.Sprintf("rated session %d (%s, 50m, q%d) ★5\n", last.ID, whenOf(last), qOf(last))
	if code != 0 || out != want {
		t.Fatalf("rate by id: %d %q, want %q", code, out, want)
	}
	if r := sessionRow(t, te, last.ID); r.Rating != 5 || r.Note != "good run" {
		t.Fatalf("stored = %+v", r)
	}
	// the older id with a note that starts with a digit word
	code, out = te.run(t, "", "session", "rate", "last", "2", "3", "sleepy")
	if code != 0 || !strings.HasSuffix(out, "★2 · note saved\n") {
		t.Fatalf("rate last 2: %d %q", code, out)
	}
	if r := sessionRow(t, te, last.ID); r.Rating != 2 || r.Note != "3 sleepy" {
		t.Fatalf("stored = %+v", r)
	}
	if r := sessionRow(t, te, old.ID); r.Rating != 0 {
		t.Fatalf("older session rated: %+v", r)
	}

	code, _ = te.run(t, "", "session", "rate", "7")
	if code != 2 || !strings.HasPrefix(te.errb.String(), "tock: session rate: rating \"7\", want 1 to 5\nusage:") {
		t.Fatalf("rate 7: %d %q", code, te.errb.String())
	}
	for _, args := range [][]string{{"session", "rate"}, {"session", "rate", "last"}, {"session", "rate", "0"}, {"session", "rate", "x"}} {
		if code, _ := te.run(t, "", args...); code != 2 {
			t.Errorf("%q: code %d, want 2", args, code)
		}
	}
	if code, _ := te.run(t, "", "session", "rate", "#999", "3"); code != 1 || te.errb.String() != "tock: session rate: no session 999 (tock session list shows ids)\n" {
		t.Fatalf("rate 999: %d %q", code, te.errb.String())
	}
	long := strings.Repeat("x", 212)
	before := sessionRow(t, te, last.ID)
	if code, _ := te.run(t, "", "session", "rate", "1", long); code != 1 || te.errb.String() != "tock: session rate: note is 212 characters, max 200\n" {
		t.Fatalf("long note: %d %q", code, te.errb.String())
	}
	if sessionRow(t, te, last.ID) != before {
		t.Fatalf("a refused note stored something")
	}
}

func TestSessionRateNoFinished(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	if code, _ := te.run(t, "", "session", "rate", "4"); code != 1 || te.errb.String() != "tock: session rate: no finished session yet (tock session start 50)\n" {
		t.Fatalf("none: %d %q", code, te.errb.String())
	}
	// a running session is never "last"
	if code, _ := te.run(t, "", "session", "start", "50"); code != 0 {
		t.Fatal(code)
	}
	if code, _ := te.run(t, "", "session", "rate", "4"); code != 1 || te.errb.String() != "tock: session rate: no finished session yet (tock session start 50)\n" {
		t.Fatalf("running only: %d %q", code, te.errb.String())
	}
	st, err := store.Open(te.dbPath())
	if err != nil {
		t.Fatal(err)
	}
	open, _ := st.OpenSession()
	st.Close()
	id := "#" + strconv.FormatInt(open.ID, 10)
	if code, _ := te.run(t, "", "session", "rate", id, "4"); code != 1 || te.errb.String() != "tock: session rate: session still running\n" {
		t.Fatalf("rate running: %d %q", code, te.errb.String())
	}
	if code, _ := te.run(t, "", "session", "note", id, "hi"); code != 1 || te.errb.String() != "tock: session note: session still running\n" {
		t.Fatalf("note running: %d %q", code, te.errb.String())
	}
}

func TestSessionNote(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	x := addSession(t, te, "10:00", 0, 50, 40, 1, store.SessionManual)
	id := strconv.FormatInt(x.ID, 10)
	if code, out := te.run(t, "", "session", "note", "#"+id, "derivation", "went", "well"); code != 0 || out != "noted session "+id+": derivation went well\n" {
		t.Fatalf("note by id: %d %q", code, out)
	}
	if code, out := te.run(t, "", "session", "note", "--id", id, "two\nwords"); code != 0 || out != "noted session "+id+": two words\n" {
		t.Fatalf("note newline: %d %q", code, out)
	}
	if code, out := te.run(t, "", "session", "note", "no", "id", "given"); code != 0 || out != "noted session "+id+": no id given\n" {
		t.Fatalf("note default last: %d %q", code, out)
	}
	if code, out := te.run(t, "", "session", "note", "#"+id); code != 0 || out != "cleared the note on session "+id+"\n" {
		t.Fatalf("clear: %d %q", code, out)
	}
	if r := sessionRow(t, te, x.ID); r.Note != "" {
		t.Fatalf("note = %q", r.Note)
	}
	if code, out := te.run(t, "", "session", "note", "#"+id, "a\x1b[2Jb"); code != 0 || out != "noted session "+id+": a?[2Jb\n" {
		t.Fatalf("escape: %d %q", code, out)
	}
	if code, _ := te.run(t, "", "session", "note", "#"+id, strings.Repeat("y", 201)); code != 1 || te.errb.String() != "tock: session note: note is 201 characters, max 200\n" {
		t.Fatalf("long: %d %q", code, te.errb.String())
	}
	if code, _ := te.run(t, "", "session", "note", "#999", "x"); code != 1 || te.errb.String() != "tock: session note: no session 999 (tock session list shows ids)\n" {
		t.Fatalf("unknown: %d %q", code, te.errb.String())
	}
	te.run(t, "", "session", "note", "#"+id, "again")
	if code, out := te.run(t, "", "session", "note"); code != 0 || out != "cleared the note on session "+id+"\n" {
		t.Fatalf("bare note clears last: %d %q", code, out)
	}
}

func TestSessionList(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	if code, out := te.run(t, "", "session", "list"); code != 0 || out != "no sessions this week\n" {
		t.Fatalf("empty: %d %q", code, out)
	}
	a := addSession(t, te, "08:30", 0, 25, 15, 2, store.SessionManual)
	b := addSession(t, te, "10:00", 0, 50, 42, 1, store.SessionManual)
	c := addSession(t, te, "09:00", 1, 40, 38, 0, store.SessionAuto) // Friday... testNow is Saturday
	if code, _ := te.run(t, "", "session", "rate", "#"+strconv.FormatInt(b.ID, 10), "4", "derivation went well"); code != 0 {
		t.Fatal(code, te.errb.String())
	}
	code, out := te.run(t, "", "session", "list")
	if code != 0 {
		t.Fatalf("list: %d", code)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 4 || !strings.HasPrefix(lines[0], "id  when") {
		t.Fatalf("list = %q", out)
	}
	ids := []int64{b.ID, a.ID, c.ID}
	for i, s := range ids {
		f := strings.Fields(lines[i+1])
		if f[0] != strconv.FormatInt(s, 10) {
			t.Errorf("row %d starts %q, want id %d", i, f[0], s)
		}
		if strings.HasSuffix(lines[i+1], " ") {
			t.Errorf("trailing space in %q", lines[i+1])
		}
	}
	rowB := strings.Join(strings.Fields(lines[1]), " ")
	if !strings.Contains(rowB, fmt.Sprintf("manual 50m 84%% %d 4 1 derivation went well", qOf(b))) {
		t.Errorf("row b = %q", rowB)
	}
	rowA := strings.Join(strings.Fields(lines[2]), " ")
	if !strings.Contains(rowA, fmt.Sprintf("manual 25m 60%% %d - 2", qOf(a))) {
		t.Errorf("row a = %q", rowA)
	}
	rowC := strings.Join(strings.Fields(lines[3]), " ")
	if !strings.Contains(rowC, fmt.Sprintf("auto 40m 95%% %d - -", qOf(c))) {
		t.Errorf("row c = %q", rowC)
	}

	// a long note is cut to 80 cells
	if code, _ := te.run(t, "", "session", "note", "#"+strconv.FormatInt(b.ID, 10), strings.Repeat("wide note ", 20)); code != 0 {
		t.Fatal(code, te.errb.String())
	}
	_, out = te.run(t, "", "session", "list")
	for _, l := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if ansi.StringWidth(l) > 80 {
			t.Errorf("line is %d cells: %q", ansi.StringWidth(l), l)
		}
	}
	if !strings.Contains(out, "…\n") {
		t.Errorf("no cut marker in %q", out)
	}
}

func TestSessionListOtherWeek(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	addSession(t, te, "10:00", 8, 50, 40, 0, store.SessionManual) // last week
	if code, out := te.run(t, "", "session", "list"); code != 0 || out != "no sessions this week\n" {
		t.Fatalf("%d %q", code, out)
	}
}

func TestSessionWeights(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	var ids []string
	for i := 0; i < 10; i++ {
		x := addSession(t, te, fmt.Sprintf("%02d:00", i), 0, 50, 5+i*4, i%4, store.SessionManual)
		ids = append(ids, strconv.FormatInt(x.ID, 10))
	}
	for i := 0; i < 6; i++ {
		if code, _ := te.run(t, "", "session", "rate", "#"+ids[i], strconv.Itoa(1+i%5)); code != 0 {
			t.Fatal(code, te.errb.String())
		}
	}
	code, out := te.run(t, "", "session", "weights")
	want := "component  default  current\n" +
		"share         0.45     0.45\n" +
		"longest       0.25     0.25\n" +
		"calm          0.15     0.15\n" +
		"steady        0.15     0.15\n" +
		"default (6/10 rated)\n"
	if code != 0 || out != want {
		t.Fatalf("below 10: %d\n%s\nwant\n%s", code, out, want)
	}
	for i := 6; i < 10; i++ {
		if code, _ := te.run(t, "", "session", "rate", "#"+ids[i], strconv.Itoa(1+i%5)); code != 0 {
			t.Fatal(code, te.errb.String())
		}
	}
	code, out = te.run(t, "", "session", "weights")
	if code != 0 || !strings.HasSuffix(out, "based on 10 rated sessions\n") || strings.Contains(out, "default (") {
		t.Fatalf("at 10: %d %q", code, out)
	}
	if _, out2 := te.run(t, "", "session", "weights"); out2 != out {
		t.Fatalf("not reproducible:\n%s\n%s", out, out2)
	}
	if out == want {
		t.Fatalf("fitted weights equal the defaults")
	}
}

const refileTestRules = testRules + `
[[project]]
name = "Analyse 1"
category = "Study"
`

func TestSessionRefile(t *testing.T) {
	te := newTestEnv(t, refileTestRules)
	te.seed(t, daySpans()...)
	x := addSession(t, te, "09:00", 0, 50, 40, 1, store.SessionManual)

	code, out := te.run(t, "", "session", "refile", "Analyse", "1")
	want := fmt.Sprintf("refiled session %d (50m) to Analyse 1 · tock undo reverts it\n", x.ID)
	if code != 0 || out != want {
		t.Fatalf("refile: %d %q %q, want %q", code, out, te.errb.String(), want)
	}
	if code, out := te.run(t, "", "edit"); code != 0 || !strings.Contains(out, "Study / Analyse 1") {
		t.Fatalf("edit list after refile: %d %q", code, out)
	}
	if code, out := te.run(t, "", "undo"); code != 0 {
		t.Fatalf("undo: %d %q %q", code, out, te.errb.String())
	}
	if code, out := te.run(t, "", "edit"); code != 0 || strings.Contains(out, "Analyse 1") {
		t.Fatalf("edit list after undo: %d %q", code, out)
	}
	// by id and by last, with a project that is not a module
	id := strconv.FormatInt(x.ID, 10)
	for _, ref := range [][]string{{"#" + id, "ATLAS"}, {"--id", id, "ATLAS"}, {"--id=" + id, "ATLAS"}, {"last", "ATLAS"}} {
		args := append([]string{"session", "refile"}, ref...)
		want = fmt.Sprintf("refiled session %d (50m) to ATLAS · tock undo reverts it\n", x.ID)
		if code, out := te.run(t, "", args...); code != 0 || out != want {
			t.Fatalf("%q: %d %q %q", args, code, out, te.errb.String())
		}
	}

	code, _ = te.run(t, "", "session", "refile")
	if code != 2 || !strings.HasPrefix(te.errb.String(), "tock: session refile: PROJECT is required\nusage:") {
		t.Fatalf("no project: %d %q", code, te.errb.String())
	}
	if code, _ := te.run(t, "", "session", "refile", "last"); code != 2 {
		t.Fatalf("last only: %d", code)
	}
	if code, _ := te.run(t, "", "session", "refile", "Foo"); code != 1 || te.errb.String() != "tock: session refile: no project \"Foo\" (the labels tab lists them)\n" {
		t.Fatalf("unknown project: %d %q", code, te.errb.String())
	}
	if code, _ := te.run(t, "", "session", "refile", "#999", "ATLAS"); code != 1 || te.errb.String() != "tock: session refile: no session 999 (tock session list shows ids)\n" {
		t.Fatalf("unknown id: %d %q", code, te.errb.String())
	}
	te.now = testNow.Add(time.Hour)
	if code, _ := te.run(t, "", "session", "start", "25"); code != 0 {
		t.Fatalf("start: %d %q", code, te.errb.String())
	}
	if code, _ := te.run(t, "", "session", "refile", "#"+strconv.FormatInt(x.ID+1, 10), "ATLAS"); code != 1 || te.errb.String() != "tock: session refile: session still running\n" {
		t.Fatalf("running: %d %q", code, te.errb.String())
	}
}

// A leading number is never a session id: only #N and --id N name one, so a
// rating or the first word of a note cannot land on another session.
func TestSessionLeadingNumberIsNotAnID(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	old := addSession(t, te, "08:00", 0, 25, 10, 1, store.SessionManual)
	last := addSession(t, te, "10:00", 0, 50, 40, 1, store.SessionManual)
	oldRef := "#" + strconv.FormatInt(old.ID, 10)

	// `rate 4 2 hours deep` rates the last session ★4, not session 4 ★2
	if code, out := te.run(t, "", "session", "rate", "4", "2", "hours", "deep"); code != 0 || !strings.Contains(out, fmt.Sprintf("rated session %d ", last.ID)) {
		t.Fatalf("rate 4 2: %d %q %q", code, out, te.errb.String())
	}
	if r := sessionRow(t, te, last.ID); r.Rating != 4 || r.Note != "2 hours deep" {
		t.Fatalf("stored = %+v", r)
	}
	// `note 25 min of reading` is a note on the last session
	if code, out := te.run(t, "", "session", "note", "25", "min", "of", "reading"); code != 0 || out != fmt.Sprintf("noted session %d: 25 min of reading\n", last.ID) {
		t.Fatalf("note 25: %d %q %q", code, out, te.errb.String())
	}
	if r := sessionRow(t, te, last.ID); r.Note != "25 min of reading" {
		t.Fatalf("stored = %+v", r)
	}
	// `note 42` alone is the note "42", it does not clear session 42
	if code, out := te.run(t, "", "session", "note", "42"); code != 0 || out != fmt.Sprintf("noted session %d: 42\n", last.ID) {
		t.Fatalf("note 42: %d %q", code, out)
	}
	// the word "last" in a note is text, not a target
	if code, out := te.run(t, "", "session", "note", "last", "night", "ran", "long"); code != 0 || out != fmt.Sprintf("noted session %d: last night ran long\n", last.ID) {
		t.Fatalf("note last night: %d %q", code, out)
	}
	// both unambiguous id forms still pick the older session
	if code, out := te.run(t, "", "session", "rate", oldRef, "3"); code != 0 || !strings.Contains(out, fmt.Sprintf("rated session %d ", old.ID)) {
		t.Fatalf("rate by #id: %d %q", code, out)
	}
	if code, out := te.run(t, "", "session", "note", "--id", strconv.FormatInt(old.ID, 10), "25", "min"); code != 0 || out != fmt.Sprintf("noted session %d: 25 min\n", old.ID) {
		t.Fatalf("note by --id: %d %q", code, out)
	}
	if r := sessionRow(t, te, old.ID); r.Rating != 3 || r.Note != "25 min" {
		t.Fatalf("older = %+v", r)
	}
	if r := sessionRow(t, te, last.ID); r.Rating != 4 || r.Note != "last night ran long" {
		t.Fatalf("last changed: %+v", r)
	}
	// a project made of digits is a project, not an id
	if code, _ := te.run(t, "", "session", "refile", "2024"); code != 1 || te.errb.String() != "tock: session refile: no project \"2024\" (the labels tab lists them)\n" {
		t.Fatalf("refile 2024: %d %q", code, te.errb.String())
	}
	// --id needs a number
	for _, args := range [][]string{{"session", "note", "--id"}, {"session", "note", "--id", "x", "hi"}, {"session", "rate", "--id=", "4"}} {
		if code, _ := te.run(t, "", args...); code != 2 {
			t.Errorf("%q: code %d, want 2 (%q)", args, code, te.errb.String())
		}
	}
}

func TestSessionSnooze(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)

	if code, _ := te.run(t, "", "session", "snooze"); code != 1 || te.errb.String() != "tock: session snooze: no session running (tock session start 50)\n" {
		t.Fatalf("no session: %d %q", code, te.errb.String())
	}
	if code, _ := te.run(t, "", "session", "start", "50"); code != 0 {
		t.Fatalf("start: %d %q", code, te.errb.String())
	}
	for _, c := range []struct {
		args []string
		code int
		out  string
	}{
		{[]string{"snooze"}, 0, "distraction notices snoozed for 10 min, until 15:10\n"},
		{[]string{"snooze", "25"}, 0, "distraction notices snoozed for 25 min, until 15:25\n"},
		{[]string{"snooze", "off"}, 0, "distraction notices back on\n"},
		{[]string{"snooze", "off"}, 0, "distraction notices were not snoozed\n"},
	} {
		if code, out := te.run(t, "", append([]string{"session"}, c.args...)...); code != c.code || out != c.out {
			t.Errorf("%q: %d %q, want %d %q (stderr %q)", c.args, code, out, c.code, c.out, te.errb.String())
		}
	}
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"snooze", "300"}, "tock: session snooze: 300 min is out of range, want 1 to 240\n"},
		{[]string{"snooze", "0"}, "tock: session snooze: 0 min is out of range, want 1 to 240\n"},
		{[]string{"snooze", "abc"}, "tock: session snooze: \"abc\", want minutes or off\n"},
		{[]string{"snooze", "5", "6"}, "tock: session snooze takes minutes or off\n"},
	} {
		code, out := te.run(t, "", append([]string{"session"}, c.args...)...)
		if code != 2 || out != "" || !strings.HasPrefix(te.errb.String(), c.want) {
			t.Errorf("%q: %d %q stderr %q, want exit 2 and %q", c.args, code, out, te.errb.String(), c.want)
		}
	}
}

func TestSessionSnoozeUsesConfig(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	if err := os.WriteFile(filepath.Join(te.ConfigDir, "config.toml"), []byte("[focus]\nsnooze = 20\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	te.run(t, "", "session", "start", "50")
	if code, out := te.run(t, "", "session", "snooze"); code != 0 || out != "distraction notices snoozed for 20 min, until 15:20\n" {
		t.Fatalf("snooze: %d %q %q", code, out, te.errb.String())
	}
}

// --paper marks the session as paper work: idle never counts as off task.
func TestSessionStartPaper(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	heartbeat(t, te)
	if code, out := te.run(t, "", "session", "start", "50", "--paper"); code != 0 || out != "session started: 50 min, ends 15:50, on paper\n" {
		t.Fatalf("start: %d %q %q", code, out, te.errb.String())
	}
	te.run(t, "", "session", "stop")
	if code, out := te.run(t, "", "session", "start", "--paper"); code != 0 || !strings.Contains(out, "on paper") {
		t.Fatalf("start --paper: %d %q %q", code, out, te.errb.String())
	}
	if code, _ := te.run(t, "", "session", "start", "--pen"); code != 2 {
		t.Fatalf("bad flag: exit %d", code)
	}
}

func TestBreakCommand(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	if code, out := te.run(t, "", "break"); code != 0 || out != "break: 5 min, until 15:05\n" {
		t.Fatalf("break: %d %q %q", code, out, te.errb.String())
	}
	if code, out := te.run(t, "", "break", "15"); code != 0 || out != "break: 15 min, until 15:15\n" {
		t.Fatalf("break 15: %d %q %q", code, out, te.errb.String())
	}
	for _, bad := range []string{"0", "121", "x"} {
		if code, _ := te.run(t, "", "break", bad); code != 2 {
			t.Fatalf("break %s: exit %d", bad, code)
		}
	}
}

// A session that showed wake screens says so in the list.
func TestSessionListWoken(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	st, err := store.Open(te.dbPath())
	if err != nil {
		t.Fatal(err)
	}
	s, err := st.StartSession(at(t, "10:00"), at(t, "10:50"))
	if err != nil {
		t.Fatal(err)
	}
	s.Wakes, s.WakesBack, s.EndMS = 2, 1, at(t, "10:50")
	if _, err := st.UpdateSession(s); err != nil {
		t.Fatal(err)
	}
	if _, err := st.EndSession(s); err != nil {
		t.Fatal(err)
	}
	st.Close()
	if code, out := te.run(t, "", "session", "list"); code != 0 || !strings.Contains(out, "woken 2× · back 1×") {
		t.Fatalf("list: %d %q", code, out)
	}
}

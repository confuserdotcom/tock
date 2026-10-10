package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tock/internal/apptest"
	"tock/internal/classify"
)

// twoMachines is the seeded day on "mac" and 30 min of tock work on "ryuk"
// while the Mac was idle (11:45–12:15), both syncing through dir.
func twoMachines(t *testing.T) (mac, ryuk *App, dir string) {
	t.Helper()
	dir = t.TempDir()
	em := apptest.New(t, apptest.Rules)
	em.Seed(t, apptest.DaySpans()...)
	em.WriteConfig(t, "[sync]\ndir = \""+dir+"\"\nmachine = \"mac\"\n")
	er := apptest.New(t, apptest.Rules)
	er.Seed(t, apptest.Span{ID: 1, Start: "11:45", End: "12:15", Bundle: "term", App: "WezTerm", Title: "tock │ ryuk"})
	er.WriteConfig(t, "[sync]\ndir = \""+dir+"\"\nmachine = \"ryuk\"\n")
	return open(t, em), open(t, er), dir
}

func TestSyncMergesTheOtherMachine(t *testing.T) {
	mac, ryuk, dir := twoMachines(t)
	before, err := mac.Day(mac.now())
	if err != nil {
		t.Fatal(err)
	}
	r, err := ryuk.Sync(SyncDays)
	if err != nil || r.Machine != "ryuk" || r.Written != 2 {
		t.Fatalf("ryuk sync = %+v, %v", r, err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "ryuk", "2026-10-10.jsonl"))
	if err != nil || !strings.Contains(string(b), `"b":"term"`) || !strings.Contains(string(b), `"c":"Code","p":"tock"`) {
		t.Fatalf("ryuk day file = %s, %v", b, err)
	}
	if fi, _ := os.Stat(filepath.Join(dir, "ryuk", "2026-10-10.jsonl")); fi.Mode().Perm() != 0o600 {
		t.Fatalf("day file mode %v", fi.Mode())
	}

	r, err = mac.Sync(SyncDays)
	if err != nil || r.Read != 2 || len(r.Machines) != 1 || r.Machines[0].Name != "ryuk" {
		t.Fatalf("mac sync = %+v, %v", r, err)
	}
	d, err := mac.Day(mac.now())
	if err != nil {
		t.Fatal(err)
	}
	if d.TrackedMS != before.TrackedMS+30*60_000 {
		t.Fatalf("tracked %d, want %d + 30m", d.TrackedMS, before.TrackedMS)
	}
	var remoteID int64
	for _, row := range d.Timeline {
		if row.App == "ryuk · WezTerm" {
			remoteID = row.IDs[0]
			if row.Label.Category != "Code" || row.Label.Project != "tock" || row.Start != apptest.At(t, "11:45", 0) {
				t.Fatalf("remote row = %+v", row)
			}
		}
	}
	if !IsRemoteView(remoteID) {
		t.Fatalf("no remote row in %+v", d.Timeline)
	}
	if err := mac.Tag([]int64{remoteID}, classify.Want{Category: "Communication"}); !errors.Is(err, ErrRemote) {
		t.Fatalf("tagging a remote row = %v", err)
	}

	// Nothing changed: nothing written or read; the Mac's file never holds
	// ryuk's time.
	if r, err := mac.Sync(SyncDays); err != nil || r.Written != 0 || r.Read != 0 {
		t.Fatalf("second sync = %+v, %v", r, err)
	}
	b, _ = os.ReadFile(filepath.Join(dir, "mac", "2026-10-10.jsonl"))
	if strings.Contains(string(b), "ryuk") || strings.Contains(string(b), "tock.idle") || !strings.Contains(string(b), "WezTerm") {
		t.Fatalf("mac day file:\n%s", b)
	}
}

func TestSyncOffAndBadLines(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	a := open(t, e)
	if _, err := a.Sync(SyncDays); !errors.Is(err, ErrSyncOff) {
		t.Fatalf("sync off = %v", err)
	}
	p := filepath.Join(t.TempDir(), "2026-10-10.jsonl")
	os.WriteFile(p, []byte("{\"s\":1,\"e\":2,\"b\":\"x\",\"a\":\"X\"}\nnot json\n{\"s\":5,\"e\":5,\"b\":\"x\"}\n{\"s\":1,\"e\":9,\"b\":\"\"}\n"), 0o600)
	vs, err := readDayFile(p)
	if err != nil || len(vs) != 1 || vs[0].App != "X" {
		t.Fatalf("parsed = %+v, %v", vs, err)
	}
}

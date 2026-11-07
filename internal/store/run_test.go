package store

import (
	"strconv"
	"testing"
)

func TestRunStartTouchEnd(t *testing.T) {
	s := openAt(t, dbPath(t))
	defer s.Close()
	id, err := s.StartRun(Run{StartMS: 1000, State: RunRecording, BootMS: 50, PID: 7, Version: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.TouchRun(id, 61_000); err != nil {
		t.Fatal(err)
	}
	if err := s.TouchRun(id, 30_000); err != nil { // never moves back
		t.Fatal(err)
	}
	rs, err := s.Runs(0, 100_000)
	if err != nil {
		t.Fatal(err)
	}
	want := Run{ID: id, StartMS: 1000, LastMS: 61_000, State: RunRecording, BootMS: 50, PID: 7, Version: "v1"}
	if len(rs) != 1 || rs[0] != want {
		t.Fatalf("runs = %+v, want [%+v]", rs, want)
	}
	if err := s.EndRun(id, 65_000, RunSignal); err != nil {
		t.Fatal(err)
	}
	rs, _ = s.Runs(0, 100_000)
	if rs[0].StopMS != 65_000 || rs[0].LastMS != 65_000 || rs[0].Reason != RunSignal {
		t.Fatalf("after end: %+v", rs[0])
	}
}

// A row left open by a trackd that died is closed as a crash when the next
// run starts, at the later of its last minute tick and the heartbeat.
func TestStartRunClosesCrashed(t *testing.T) {
	s := openAt(t, dbPath(t))
	defer s.Close()
	old, _ := s.StartRun(Run{StartMS: 1000, State: RunRecording})
	if err := s.TouchRun(old, 60_000); err != nil {
		t.Fatal(err)
	}
	// the 10 s heartbeat ran on for 40 s after the last minute tick
	if err := s.SetState(KeyHeartbeat, strconv.Itoa(100_000), 100_000); err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartRun(Run{StartMS: 400_000, State: RunRecording}); err != nil {
		t.Fatal(err)
	}
	rs, _ := s.Runs(0, 500_000)
	if len(rs) != 2 {
		t.Fatalf("runs = %+v", rs)
	}
	if rs[0].StopMS != 100_000 || rs[0].Reason != RunCrash {
		t.Fatalf("crashed run = %+v, want stop 100000 crash", rs[0])
	}
	if rs[1].StopMS != 0 || rs[1].Reason != "" {
		t.Fatalf("new run = %+v, want open", rs[1])
	}
}

// A heartbeat far past the last tick (written by a later process, or a
// clock jump) cannot stretch the crashed run beyond one tick period.
func TestStartRunCrashStopBounded(t *testing.T) {
	s := openAt(t, dbPath(t))
	defer s.Close()
	old, _ := s.StartRun(Run{StartMS: 1000, State: RunRecording})
	_ = s.TouchRun(old, 60_000)
	_ = s.SetState(KeyHeartbeat, strconv.Itoa(5_000_000), 5_000_000)
	if _, err := s.StartRun(Run{StartMS: 6_000_000, State: RunRecording}); err != nil {
		t.Fatal(err)
	}
	rs, _ := s.Runs(0, 7_000_000)
	if rs[0].StopMS != 60_000+RunTickMS+RunSlackMS {
		t.Fatalf("crashed run stop = %d, want %d", rs[0].StopMS, 60_000+RunTickMS+RunSlackMS)
	}
	// a heartbeat from before the run started is not its own
	s2 := openAt(t, dbPath(t))
	defer s2.Close()
	_ = s2.SetState(KeyHeartbeat, "500", 500)
	r, _ := s2.StartRun(Run{StartMS: 1000, State: RunRecording})
	_ = s2.TouchRun(r, 2000)
	_, _ = s2.StartRun(Run{StartMS: 9000, State: RunRecording})
	rs, _ = s2.Runs(0, 10_000)
	if rs[0].StopMS != 2000 {
		t.Fatalf("stop = %d, want the last tick 2000", rs[0].StopMS)
	}
}

// Runs returns the rows overlapping the range plus the one before and the
// one after, which say what the gaps at its edges were.
func TestRunsRangeHasNeighbours(t *testing.T) {
	s := openAt(t, dbPath(t))
	defer s.Close()
	for _, r := range [][2]int64{{0, 100}, {200, 300}, {400, 500}, {600, 700}, {800, 900}} {
		id, err := s.StartRun(Run{StartMS: r[0], State: RunRecording})
		if err != nil {
			t.Fatal(err)
		}
		if err := s.EndRun(id, r[1], RunSignal); err != nil {
			t.Fatal(err)
		}
	}
	rs, err := s.Runs(450, 550)
	if err != nil {
		t.Fatal(err)
	}
	var starts []int64
	for _, r := range rs {
		starts = append(starts, r.StartMS)
	}
	if len(starts) != 3 || starts[0] != 200 || starts[1] != 400 || starts[2] != 600 {
		t.Fatalf("starts = %v, want [200 400 600]", starts)
	}
	rs, _ = s.Runs(310, 390) // inside a gap: just its two sides
	if len(rs) != 2 || rs[0].StartMS != 200 || rs[1].StartMS != 400 {
		t.Fatalf("gap range = %+v", rs)
	}
	rs, _ = s.Runs(2000, 3000) // after everything: the last row
	if len(rs) != 1 || rs[0].StartMS != 800 {
		t.Fatalf("late range = %+v", rs)
	}
}

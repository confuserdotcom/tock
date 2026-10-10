package store

import (
	"path/filepath"
	"testing"
)

func TestRemoteViews(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "tock.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, ok, err := s.RemoteFileSeen("ryuk", "2026-10-10"); ok || err != nil {
		t.Fatalf("seen before reading: %v %v", ok, err)
	}
	title := "~/tock"
	day1 := []RemoteView{
		{StartMS: 100, EndMS: 200, Bundle: "kitty", App: "kitty", Title: &title, Category: "Code", Project: "tock"},
		{StartMS: 300, EndMS: 300, Bundle: "x", App: "x"}, // empty: dropped
	}
	if err := s.ReplaceRemoteDay("ryuk", "2026-10-10", RemoteFile{MtimeMS: 5, Size: 10}, day1); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceRemoteDay("ryuk", "2026-10-09", RemoteFile{MtimeMS: 4, Size: 3},
		[]RemoteView{{StartMS: 10, EndMS: 50, Bundle: "firefox", App: "firefox"}}); err != nil {
		t.Fatal(err)
	}
	vs, err := s.RemoteViews(0, 1000)
	if err != nil || len(vs) != 2 || vs[0].Bundle != "firefox" || vs[0].Title != nil || *vs[1].Title != "~/tock" ||
		vs[1].Machine != "ryuk" || vs[1].Category != "Code" {
		t.Fatalf("views = %+v, %v", vs, err)
	}
	if vs, _ := s.RemoteViews(150, 160); len(vs) != 1 || vs[0].StartMS != 100 {
		t.Fatalf("overlap = %+v", vs)
	}
	n0, id0, _ := s.RemoteStamp()
	if err := s.ReplaceRemoteDay("ryuk", "2026-10-10", RemoteFile{MtimeMS: 6, Size: 12},
		[]RemoteView{{StartMS: 100, EndMS: 250, Bundle: "kitty", App: "kitty"}}); err != nil {
		t.Fatal(err)
	}
	if n, id, _ := s.RemoteStamp(); n != n0 || id == id0 {
		t.Fatalf("stamp did not move: %d/%d vs %d/%d", n, id, n0, id0)
	}
	if f, ok, _ := s.RemoteFileSeen("ryuk", "2026-10-10"); !ok || f != (RemoteFile{MtimeMS: 6, Size: 12}) {
		t.Fatalf("seen = %+v %v", f, ok)
	}
	ms, err := s.RemoteMachines()
	if err != nil || len(ms) != 1 || ms[0] != (RemoteMachine{Name: "ryuk", LastMS: 250, Days: 2}) {
		t.Fatalf("machines = %+v, %v", ms, err)
	}
}

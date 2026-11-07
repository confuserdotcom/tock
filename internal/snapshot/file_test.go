package snapshot

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// liveDB makes a small SQLite file with one table and n rows.
func liveDB(t *testing.T, n int) (*sql.DB, string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "tock.db")
	db, err := sql.Open("sqlite", "file:"+p)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`CREATE TABLE span(id INTEGER PRIMARY KEY, title TEXT)`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < n; i++ {
		if _, err := db.Exec(`INSERT INTO span(title) VALUES('x')`); err != nil {
			t.Fatal(err)
		}
	}
	return db, p
}

func vacuum(db *sql.DB) func(string) error {
	return func(p string) error { _, err := db.Exec(`VACUUM INTO ?`, p); return err }
}

func plenty(string) (uint64, error) { return 1 << 40, nil }

func TestWriteListExtract(t *testing.T) {
	db, _ := liveDB(t, 50)
	dir := filepath.Join(t.TempDir(), "auto")
	name := DailyName(1_700_000_000_000, 23)
	info, err := Write(Opts{Dir: dir, Name: name, LiveSize: 1, Free: plenty, Vacuum: vacuum(db)})
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != name || info.Size == 0 || info.Kind != Daily {
		t.Fatalf("info = %+v", info)
	}
	for _, p := range []string{dir, info.Path} {
		fi, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		want := os.FileMode(0o600)
		if fi.IsDir() {
			want = 0o700
		}
		if fi.Mode().Perm() != want {
			t.Errorf("%s mode %v, want %v", p, fi.Mode().Perm(), want)
		}
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("dir holds %d files, want only the snapshot", len(entries))
	}
	list, err := List(dir)
	if err != nil || len(list) != 1 || list[0].Path != info.Path {
		t.Fatalf("List = %+v %v", list, err)
	}
	dst := filepath.Join(t.TempDir(), "back.db")
	if err := Extract(info.Path, dst); err != nil {
		t.Fatal(err)
	}
	back, err := sql.Open("sqlite", "file:"+dst+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer back.Close()
	var n int
	if err := back.QueryRow(`SELECT count(*) FROM span`).Scan(&n); err != nil || n != 50 {
		t.Fatalf("rows = %d %v, want 50", n, err)
	}
}

func TestWriteLowDisk(t *testing.T) {
	db, _ := liveDB(t, 1)
	dir := t.TempDir()
	_, err := Write(Opts{Dir: dir, Name: DailyName(1, 1), LiveSize: 100, Free: func(string) (uint64, error) { return 150, nil }, Vacuum: vacuum(db)})
	if !errors.Is(err, ErrLowDisk) {
		t.Fatalf("err = %v, want ErrLowDisk", err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("low disk left %d files", len(entries))
	}
}

func TestWriteVacuumFailsCleansUp(t *testing.T) {
	dir := t.TempDir()
	_, err := Write(Opts{Dir: dir, Name: DailyName(1, 1), Free: plenty, Vacuum: func(string) error { return errors.New("database disk image is malformed") }})
	var d *DamagedError
	if !errors.As(err, &d) {
		t.Fatalf("err = %v, want DamagedError", err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("failed write left %d files", len(entries))
	}
}

func TestExtractCorrupt(t *testing.T) {
	src := filepath.Join(t.TempDir(), DailyName(1, 1))
	if err := os.WriteFile(src, []byte("not gzip"), 0o600); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "x.db")
	if err := Extract(src, dst); err == nil {
		t.Fatal("corrupt snapshot extracted")
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Fatal("corrupt extract left a file")
	}
}

func TestListIgnoresOthers(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"notes.txt", ".daily-x.db.tmp", PreName(22, 23)} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	list, err := List(dir)
	if err != nil || len(list) != 1 || list[0].Kind != Pre || list[0].AtMS == 0 {
		t.Fatalf("List = %+v %v", list, err)
	}
	if l, err := List(filepath.Join(dir, "missing")); err != nil || l != nil {
		t.Fatalf("missing dir: %v %v", l, err)
	}
}

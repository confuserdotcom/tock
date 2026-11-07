package store

import (
	"compress/gzip"
	"database/sql"
	"io"
	"os"
	"path/filepath"
	"testing"

	"tock/internal/snapshot"
)

// TestUpgradeFixtures opens databases written by released versions and
// checks that HEAD migrates them without losing rows.
func TestUpgradeFixtures(t *testing.T) {
	fixtures, _ := filepath.Glob("testdata/upgrade/v*.db.gz")
	if len(fixtures) == 0 {
		t.Fatal("no fixtures in testdata/upgrade")
	}
	for _, f := range fixtures {
		t.Run(filepath.Base(f), func(t *testing.T) {
			path := dbPath(t)
			gunzip(t, f, path)
			raw, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
			if err != nil {
				t.Fatal(err)
			}
			from, err := currentVersion(raw)
			raw.Close()
			if err != nil {
				t.Fatal(err)
			}
			s, err := Open(path)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			v, _ := s.SchemaVersion()
			if v != LatestVersion() {
				t.Fatalf("schema = %d, want %d", v, LatestVersion())
			}
			var n, open int
			var total int64
			if err := s.db.QueryRow(`SELECT count(*), count(*) - count("end"), COALESCE(SUM("end" - start), 0) FROM span`).Scan(&n, &open, &total); err != nil {
				t.Fatal(err)
			}
			if n != 30 || open != 1 || total != 29*500_000 {
				t.Fatalf("spans = %d (open %d, total %d ms), want 30 (1, %d)", n, open, total, 29*500_000)
			}
			s.Close()
			s2, err := Open(path) // second open is a no-op
			if err != nil {
				t.Fatal(err)
			}
			s2.Close()
			// a fixture already at the latest schema needs no migration,
			// so no pre-migration copy
			list, _ := snapshot.List(AutoDir(path))
			want := 0
			if from < LatestVersion() {
				want = 1
			}
			if len(list) != want {
				t.Fatalf("pre-migration snapshots = %d, want %d", len(list), want)
			}
		})
	}
}

// gunzip writes the gzipped file from to the path to.
func gunzip(t *testing.T, from, to string) {
	t.Helper()
	in, err := os.Open(from)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	zr, err := gzip.NewReader(in)
	if err != nil {
		t.Fatal(err)
	}
	out, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(out, zr); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
}

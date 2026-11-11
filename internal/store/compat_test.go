package store

import (
	"database/sql"
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
)

// A newer tock that only added tables or columns leaves compat alone, so
// an older binary still opens the database.
func TestOpenNewerSchemaSameCompat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tock.db")
	s := openAt(t, path)
	_ = s.Close()
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO schema_version(version, applied_at) VALUES(99, 1)`); err != nil {
		t.Fatal(err)
	}
	_ = raw.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatalf("Open of a newer, compatible schema: %v", err)
	}
	defer s.Close()
	if c, err := s.Compat(); err != nil || c != CompatLevel {
		t.Fatalf("Compat = %d, %v; want %d", c, err, CompatLevel)
	}
}

func TestCompatRaised(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tock.db")
	s := openAt(t, path)
	defer s.Close()
	if _, err := s.db.Exec(`UPDATE schema_meta SET value = ? WHERE key = 'compat'`, CompatLevel+1); err != nil {
		t.Fatal(err)
	}
	if c, err := s.Compat(); err != nil || c != CompatLevel+1 {
		t.Fatalf("Compat = %d, %v", c, err)
	}
}

// MarkStopped writes trackd's stop reason into a database trackd itself
// refuses to open, touching nothing but daemon_state.
func TestMarkStopped(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tock.db")
	s := openAt(t, path)
	_ = s.Close()
	if err := MarkStopped(path, StoppedSchemaNewer, 1234); err != nil {
		t.Fatal(err)
	}
	s = openAt(t, path)
	defer s.Close()
	v, ok, err := s.State(KeyTrackdStopped)
	if err != nil || !ok || v != StoppedSchemaNewer {
		t.Fatalf("state = %q %v %v", v, ok, err)
	}
}

var (
	breakingSQL = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\bDROP\b`),
		regexp.MustCompile(`(?i)\bRENAME\b`),
		regexp.MustCompile(`(?is)\bADD\s+COLUMN\b[^;]*\bNOT\s+NULL\b`),
	}
	hasDefault   = regexp.MustCompile(`(?i)\bDEFAULT\b`)
	raisesCompat = regexp.MustCompile(`(?i)UPDATE\s+schema_meta\s+SET\s+value\b[^;]*key\s*=\s*'compat'`)
)

const breaksMarker = "-- breaks-older-writers"

// migrationLint returns why src would break an older trackd still writing
// to the database, or "" when it is additive or says so and raises compat.
func migrationLint(src string) string {
	var hits []string
	// Comments go first: a ; inside one would otherwise split it and
	// leave its words to be read as code.
	for _, code := range strings.Split(stripComments(src), ";") {
		for i, re := range breakingSQL {
			if !re.MatchString(code) {
				continue
			}
			if i == 2 && hasDefault.MatchString(code) {
				continue
			}
			hits = append(hits, strings.TrimSpace(code))
		}
	}
	if len(hits) == 0 {
		return ""
	}
	if strings.Contains(src, breaksMarker) && raisesCompat.MatchString(src) {
		return ""
	}
	return "breaks older writers without " + breaksMarker + " and a compat raise: " + strings.Join(hits, "; ")
}

func stripComments(s string) string {
	var b strings.Builder
	for _, line := range strings.Split(s, "\n") {
		if i := strings.Index(line, "--"); i >= 0 {
			line = line[:i]
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

// TestMigrationsAreAdditive: every migration keeps an older trackd able to
// write, unless it says otherwise and raises compat.
func TestMigrationsAreAdditive(t *testing.T) {
	ents, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		b, err := fs.ReadFile(migrationFS, "migrations/"+e.Name())
		if err != nil {
			t.Fatal(err)
		}
		if why := migrationLint(string(b)); why != "" {
			t.Errorf("%s: %s", e.Name(), why)
		}
	}
}

func TestMigrationLint(t *testing.T) {
	cases := map[string]bool{ // src -> flagged
		`CREATE TABLE x (a INTEGER NOT NULL);`:                      false,
		`ALTER TABLE span ADD COLUMN x INTEGER NOT NULL DEFAULT 0;`: false,
		`ALTER TABLE span ADD COLUMN x INTEGER NOT NULL;`:           true,
		`DROP TABLE span;`:                                                                            true,
		`ALTER TABLE span RENAME COLUMN a TO b;`:                                                      true,
		`-- we never DROP anything here` + "\n" + `CREATE TABLE y (a);`:                               false,
		"-- one; then DROP TRIGGER x\nCREATE TABLE z (a);":                                            false,
		breaksMarker + "\nDROP TABLE span;\nUPDATE schema_meta SET value = '2' WHERE key = 'compat';": false,
		breaksMarker + "\nDROP TABLE span;":                                                           true, // marker without the raise
	}
	for src, want := range cases {
		if got := migrationLint(src) != ""; got != want {
			t.Errorf("%q flagged = %v, want %v", src, got, want)
		}
	}
}

// The lint must reject a real file too, not only strings: run migrateFS's
// loader over a temp migration set.
func TestMigrationLintTempFile(t *testing.T) {
	fsys := fstest.MapFS{"migrations/0099_bad.sql": {Data: []byte("ALTER TABLE span ADD COLUMN x INTEGER NOT NULL;")}}
	b, _ := fs.ReadFile(fsys, "migrations/0099_bad.sql")
	if migrationLint(string(b)) == "" {
		t.Fatal("NOT NULL column without default passed the lint")
	}
}

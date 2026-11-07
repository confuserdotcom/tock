package main

import (
	"strings"
	"testing"

	"tock/internal/app"
)

func collect() (func(string, string), func(string, string, string), *[]string) {
	var lines []string
	ok := func(n, d string) { lines = append(lines, "ok "+n+": "+d) }
	bad := func(n, d, f string) { lines = append(lines, "!! "+n+": "+d+" — "+f) }
	return ok, bad, &lines
}

func TestDoctorBackup(t *testing.T) {
	te := newTestEnv(t, "")
	now := te.now.UnixMilli()
	cases := []struct {
		name string
		keep int
		h    app.Health
		want []string
	}{
		{"fresh", 7, app.Health{TrackdUp: true, BackupMS: now - 3*3600_000, IntegrityMS: now - 3*3600_000, IntegrityOK: "ok"},
			[]string{"ok backup: last snapshot 3 h ago", "ok database: integrity ok, checked 3 h ago"}},
		{"stale", 7, app.Health{TrackdUp: true, BackupMS: now - 50*3600_000},
			[]string{"!! backup: last snapshot 50 h ago"}},
		{"none yet", 7, app.Health{TrackdUp: true}, []string{"ok backup: no daily snapshot yet"}},
		{"error", 7, app.Health{TrackdUp: true, BackupErr: "snapshot: not enough free disk space"},
			[]string{"!! backup: snapshot: not enough free disk space"}},
		{"damaged", 7, app.Health{TrackdUp: true, IntegrityMS: now, IntegrityOK: "page 7 broken"},
			[]string{"!! database: page 7 broken — tock restore"}},
		{"off", 0, app.Health{TrackdUp: true}, []string{"ok backup: daily snapshots off ([backup] keep = 0)"}},
	}
	for _, c := range cases {
		ok, bad, lines := collect()
		doctorBackup(te.Env, c.keep, c.h, ok, bad)
		got := strings.Join(*lines, "\n")
		for _, w := range c.want {
			if !strings.Contains(got, w) {
				t.Errorf("%s: missing %q in\n%s", c.name, w, got)
			}
		}
	}
}

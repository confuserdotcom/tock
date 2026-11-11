package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tock/internal/config"
	"tock/internal/span"
	"tock/internal/store"
)

const pauseT0 = int64(1_700_000_000_000)

// pauseRig is a pipe over a real store and a real config.toml in a temp
// config dir, driven by hand through start, feed and tickAt.
type pauseRig struct {
	t      *testing.T
	cfgDir string
	path   string
	st     *store.Store
	p      pipe
}

func newPauseRig(t *testing.T, cfg string) *pauseRig {
	t.Helper()
	r := &pauseRig{t: t, cfgDir: t.TempDir(), path: filepath.Join(t.TempDir(), "tock.db")}
	r.writeConfig(cfg)
	st, err := store.Open(r.path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	r.st = st
	r.p = pipe{m: span.New(span.DefaultConfig()), st: st, now: func() int64 { return pauseT0 }, hide: newHider(r.cfgDir)}
	return r
}

// db is a second, read-only connection to the store's file.
func (r *pauseRig) db() *sql.DB {
	r.t.Helper()
	db, err := sql.Open("sqlite", "file:"+r.path+"?mode=ro")
	if err != nil {
		r.t.Fatal(err)
	}
	r.t.Cleanup(func() { db.Close() })
	return db
}

func (r *pauseRig) writeConfig(s string) {
	r.t.Helper()
	if err := os.WriteFile(filepath.Join(r.cfgDir, config.File), []byte(s), 0o600); err != nil {
		r.t.Fatal(err)
	}
}

func (r *pauseRig) sample(ts int64, bundle, title, url string) {
	r.t.Helper()
	s := span.Sample{TS: ts, BundleID: bundle, App: bundle, Title: &title}
	if url != "" {
		s.URL = &url
	}
	if err := r.p.feed(span.Input{Sample: &s}); err != nil {
		r.t.Fatal(err)
	}
}

func (r *pauseRig) paused() string {
	r.t.Helper()
	v, _, err := r.st.State(store.KeyCapturePaused)
	if err != nil {
		r.t.Fatal(err)
	}
	return v
}

// leaked counts stored spans whose title or URL holds s.
func (r *pauseRig) leaked(s string) int {
	r.t.Helper()
	var n int
	err := r.db().QueryRow(`SELECT count(*) FROM span WHERE coalesce(title,'') LIKE ? OR coalesce(url,'') LIKE ?`,
		"%"+s+"%", "%"+s+"%").Scan(&n)
	if err != nil {
		r.t.Fatal(err)
	}
	return n
}

func (r *pauseRig) spans() int {
	r.t.Helper()
	var n int
	if err := r.db().QueryRow(`SELECT count(*) FROM span`).Scan(&n); err != nil {
		r.t.Fatal(err)
	}
	return n
}

const goodConfig = "[privacy]\nhide_apps = [\"diary\"]\nhide_sites = [\"bank.com\"]\n"

// pausedConfigs are the two ways a config.toml fails to load: a broken
// file, and keys this trackd does not know (written by a newer tock).
var pausedConfigs = map[string]string{
	"malformed":   "[privacy]\nhide_apps = [\"diary\"\n",
	"newer keys":  goodConfig + "\n[someday]\nenabled = true\n",
	"unknown key": goodConfig + "keep_everything = true\n",
}

func TestCapturePausedByBadConfig(t *testing.T) {
	for name, cfg := range pausedConfigs {
		t.Run(name, func(t *testing.T) {
			r := newPauseRig(t, cfg)
			if err := r.p.start(); err != nil {
				t.Fatal(err)
			}
			if got := r.paused(); !strings.Contains(got, "config.toml") {
				t.Fatalf("paused state after start = %q, want the config error", got)
			}
			r.sample(pauseT0, "diary", "secret diary", "")
			r.sample(pauseT0+2000, "safari", "Account", "www.bank.com/acct")
			r.sample(pauseT0+4000, "editor", "visible work", "")
			r.sample(pauseT0+30_000, "editor", "visible work", "")
			if err := r.p.flush(); err != nil {
				t.Fatal(err)
			}
			if n := r.spans(); n != 0 {
				t.Fatalf("%d spans recorded while paused, want 0", n)
			}
			for _, s := range []string{"secret", "bank.com", "visible"} {
				if r.leaked(s) != 0 {
					t.Fatalf("%q persisted while paused", s)
				}
			}
		})
	}
}

// The same samples under a good config are recorded, so the test above
// is not passing for want of spans.
func TestCaptureControlGoodConfig(t *testing.T) {
	r := newPauseRig(t, goodConfig)
	if err := r.p.start(); err != nil {
		t.Fatal(err)
	}
	r.sample(pauseT0, "diary", "secret diary", "")
	r.sample(pauseT0+2000, "safari", "Account", "www.bank.com/acct")
	r.sample(pauseT0+4000, "editor", "visible work", "")
	r.sample(pauseT0+30_000, "editor", "visible work", "")
	if err := r.p.flush(); err != nil {
		t.Fatal(err)
	}
	if r.paused() != "" || r.spans() != 3 || r.leaked("visible") != 1 {
		t.Fatalf("paused %q, spans %d, visible %d; want running, 3, 1", r.paused(), r.spans(), r.leaked("visible"))
	}
	if r.leaked("secret") != 0 || r.leaked("bank.com") != 0 {
		t.Fatal("hidden app or site stored with a good config")
	}
}

func TestCapturePauseKeepsExistingData(t *testing.T) {
	r := newPauseRig(t, goodConfig)
	if err := r.p.start(); err != nil {
		t.Fatal(err)
	}
	r.sample(pauseT0, "editor", "before the break", "")
	r.writeConfig(pausedConfigs["malformed"])
	// the next read is due a minute later; that sample is dropped
	r.sample(pauseT0+hideEvery, "diary", "secret diary", "")
	if got := r.paused(); got == "" {
		t.Fatal("not paused after the config broke")
	}
	var start int64
	var end sql.NullInt64
	var title string
	if err := r.db().QueryRow(`SELECT start, "end", title FROM span`).Scan(&start, &end, &title); err != nil {
		t.Fatal(err)
	}
	if start != pauseT0 || title != "before the break" || !end.Valid {
		t.Fatalf("existing span = start %d end %v title %q; want it kept and closed", start, end, title)
	}
	if r.leaked("secret") != 0 || r.spans() != 1 {
		t.Fatalf("spans %d, secret leaked %d", r.spans(), r.leaked("secret"))
	}
}

func TestCaptureResumesWhenConfigFixed(t *testing.T) {
	r := newPauseRig(t, pausedConfigs["newer keys"])
	if err := r.p.start(); err != nil {
		t.Fatal(err)
	}
	r.sample(pauseT0, "editor", "dropped", "")
	r.writeConfig(goodConfig)
	t1 := pauseT0 + hidePausedEvery
	r.sample(t1, "diary", "secret diary", "")
	r.sample(t1+2000, "editor", "kept", "")
	r.sample(t1+30_000, "editor", "kept", "") // the merger settles a switch after a few seconds
	if err := r.p.flush(); err != nil {
		t.Fatal(err)
	}
	if got := r.paused(); got != "" {
		t.Fatalf("still paused after the fix: %q", got)
	}
	if r.leaked("dropped") != 0 || r.leaked("secret") != 0 || r.leaked("kept") != 1 {
		t.Fatalf("dropped %d secret %d kept %d", r.leaked("dropped"), r.leaked("secret"), r.leaked("kept"))
	}
	var private int
	if err := r.db().QueryRow(`SELECT private FROM span WHERE bundle_id = 'diary'`).Scan(&private); err != nil || private != 1 {
		t.Fatalf("hidden app after resume: private %d err %v", private, err)
	}
}

// While paused trackd is alive: the heartbeat advances, so status says
// "paused", not "trackd down".
func TestCapturePausedKeepsHeartbeat(t *testing.T) {
	r := newPauseRig(t, pausedConfigs["malformed"])
	if err := r.p.start(); err != nil {
		t.Fatal(err)
	}
	if err := r.p.tickAt(pauseT0 + 30_000); err != nil {
		t.Fatal(err)
	}
	last, err := r.st.LastSeen()
	if err != nil {
		t.Fatal(err)
	}
	if last != pauseT0+30_000 {
		t.Fatalf("heartbeat = %d, want %d", last, pauseT0+30_000)
	}
}

// A good start clears a paused state left by an earlier run.
func TestCaptureStartClearsStalePause(t *testing.T) {
	r := newPauseRig(t, goodConfig)
	if err := r.st.SetState(store.KeyCapturePaused, "config.toml: old error", pauseT0-1); err != nil {
		t.Fatal(err)
	}
	if err := r.p.start(); err != nil {
		t.Fatal(err)
	}
	if got := r.paused(); got != "" {
		t.Fatalf("paused = %q after a good start", got)
	}
}

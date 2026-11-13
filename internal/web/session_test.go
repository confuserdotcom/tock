package web

import (
	"strings"
	"testing"
)

func TestSessionStartStop(t *testing.T) {
	f := newFixture(t)

	var st statusJSON
	f.get("/api/status", &st)
	if st.Session != nil {
		t.Fatalf("session before start: %+v", st.Session)
	}
	if len(st.Presets) == 0 || st.Presets[0] != 25 {
		t.Fatalf("presets = %v, want the defaults", st.Presets)
	}

	code, _, b := f.do("POST", "/api/session", testToken, map[string]any{"action": "start", "minutes": 50, "goal": "  write   the intro "})
	if code != 200 {
		t.Fatalf("start: %d %s", code, b)
	}
	f.get("/api/status", &st)
	if st.Session == nil || st.Session.Goal != "write the intro" || st.Session.PlannedMS-st.Session.StartMS != 50*60_000 {
		t.Fatalf("after start: %+v", st.Session)
	}

	code, _, b = f.do("POST", "/api/session", testToken, map[string]any{"action": "start", "minutes": 25})
	if code != 409 || !strings.Contains(string(b), "running") {
		t.Fatalf("second start: %d %s, want 409", code, b)
	}

	code, _, b = f.do("POST", "/api/session", testToken, map[string]any{"action": "stop"})
	if code != 200 {
		t.Fatalf("stop: %d %s", code, b)
	}
	f.get("/api/status", &st)
	if st.Session != nil {
		t.Fatalf("after stop: %+v", st.Session)
	}

	code, _, _ = f.do("POST", "/api/session", testToken, map[string]any{"action": "stop"})
	if code != 409 {
		t.Fatalf("stop with none: %d, want 409", code)
	}
	for _, body := range []map[string]any{{"action": "start", "minutes": 0}, {"action": "nap"}} {
		if code, _, b := f.do("POST", "/api/session", testToken, body); code != 400 {
			t.Fatalf("%v: %d %s, want 400", body, code, b)
		}
	}
}

func TestSessionFocusOff(t *testing.T) {
	f := newFixture(t)
	f.e.WriteConfig(t, "[focus]\nenabled = false\n")
	if _, err := f.a.Reload(); err != nil {
		t.Fatal(err)
	}
	if code, _, b := f.do("POST", "/api/session", testToken, map[string]any{"action": "start", "minutes": 25}); code != 409 {
		t.Fatalf("start with focus off: %d %s, want 409", code, b)
	}
}

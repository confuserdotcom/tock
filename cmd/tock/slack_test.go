package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	"tock/internal/slack"
)

func slackServer(t *testing.T, te *tenv) *[]string {
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		calls = append(calls, strings.TrimPrefix(r.URL.Path, "/")+" "+string(b))
		if r.URL.Path == "/users.profile.get" {
			w.Write([]byte(`{"ok":true,"profile":{"status_text":"In a focus session","status_emoji":":dart:"}}`))
			return
		}
		w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)
	te.NewSlack = func(tok string) *slack.Client {
		return &slack.Client{Token: tok, URL: srv.URL + "/", HTTP: srv.Client()}
	}
	return &calls
}

func TestSlackLoginStartStop(t *testing.T) {
	te := newTestEnv(t, "")
	calls := slackServer(t, te)
	if code, _ := te.run(t, "nope\n", "slack", "login"); code != 1 {
		t.Fatalf("bad token accepted")
	}
	if code, _ := te.run(t, "xoxp-123\n", "slack", "login"); code != 0 {
		t.Fatalf("login: %s", te.errb)
	}
	fi, err := os.Stat(te.slackTokenPath())
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("token file = %v, %v", fi, err)
	}
	until := strconv.FormatInt(testNow.UnixMilli()+25*60_000, 10)
	// Off by default: start does nothing.
	if code, _ := te.run(t, "", "slack", "start", "--until", until); code != 0 || len(*calls) != 0 {
		t.Fatalf("start while off: %d %q", code, *calls)
	}
	if err := os.WriteFile(te.ConfigDir+"/config.toml", []byte("[slack]\nenabled = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _ := te.run(t, "", "slack", "start", "--until", until); code != 0 {
		t.Fatalf("start: %s", te.errb)
	}
	if len(*calls) != 2 || !strings.HasPrefix((*calls)[1], `dnd.setSnooze {"num_minutes":25}`) {
		t.Fatalf("start calls = %q", *calls)
	}
	*calls = nil
	if code, _ := te.run(t, "", "slack", "stop"); code != 0 || len(*calls) != 3 {
		t.Fatalf("stop: %d %q %s", code, *calls, te.errb)
	}
	code, out := te.run(t, "", "slack")
	if code != 0 || !strings.Contains(out, "slack: on") || !strings.Contains(out, "token: ok") || strings.Contains(out, "xoxp") {
		t.Fatalf("status = %d %q", code, out)
	}
	if code, _ := te.run(t, "", "slack", "logout"); code != 0 {
		t.Fatal("logout")
	}
	if _, out := te.run(t, "", "slack"); !strings.Contains(out, "token: missing") {
		t.Fatalf("after logout: %q", out)
	}
}

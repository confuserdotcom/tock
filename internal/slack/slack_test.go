package slack

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fake struct {
	calls  []string
	status Profile
	fail   string
}

func (f *fake) server(t *testing.T) *Client {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer xoxp-1" {
			t.Errorf("auth = %q", r.Header.Get("Authorization"))
		}
		m := strings.TrimPrefix(r.URL.Path, "/")
		b, _ := io.ReadAll(r.Body)
		f.calls = append(f.calls, m+" "+string(b))
		if m == f.fail {
			w.Write([]byte(`{"ok":false,"error":"missing_scope"}`))
			return
		}
		switch m {
		case "users.profile.get":
			json.NewEncoder(w).Encode(map[string]any{"ok": true, "profile": f.status})
		case "users.profile.set":
			var body struct{ Profile Profile }
			json.Unmarshal(b, &body)
			f.status = body.Profile
			w.Write([]byte(`{"ok":true}`))
		default:
			w.Write([]byte(`{"ok":true}`))
		}
	}))
	t.Cleanup(srv.Close)
	return &Client{Token: "xoxp-1", URL: srv.URL + "/", HTTP: srv.Client()}
}

func TestStartStop(t *testing.T) {
	f := &fake{}
	c := f.server(t)
	ctx := context.Background()
	if err := c.Start(ctx, "Focus", ":dart:", true, 1_000_000, 1_000_000+25*60_000); err != nil {
		t.Fatal(err)
	}
	want := []string{
		`users.profile.set {"profile":{"status_text":"Focus","status_emoji":":dart:","status_expiration":2500}}`,
		`dnd.setSnooze {"num_minutes":25}`,
	}
	if strings.Join(f.calls, "\n") != strings.Join(want, "\n") {
		t.Fatalf("calls =\n%s", strings.Join(f.calls, "\n"))
	}
	f.calls = nil
	if err := c.Stop(ctx, "Focus", ":dart:", true); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 3 || !strings.HasPrefix(f.calls[1], `users.profile.set {"profile":{"status_text":""`) || !strings.HasPrefix(f.calls[2], "dnd.endSnooze") {
		t.Fatalf("stop calls = %q", f.calls)
	}
}

func TestStopKeepsAnotherStatus(t *testing.T) {
	f := &fake{status: Profile{StatusText: "Lunch", StatusEmoji: ":taco:"}}
	c := f.server(t)
	if err := c.Stop(context.Background(), "Focus", ":dart:", false); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 1 || f.status.StatusText != "Lunch" {
		t.Fatalf("calls = %q, status = %+v", f.calls, f.status)
	}
}

func TestErrorNamesCodeNotToken(t *testing.T) {
	f := &fake{fail: "dnd.setSnooze"}
	c := f.server(t)
	err := c.Start(context.Background(), "Focus", ":dart:", true, 0, 60_000)
	if err == nil || err.Error() != "slack dnd.setSnooze: missing_scope" || strings.Contains(err.Error(), "xoxp") {
		t.Fatalf("err = %v", err)
	}
}

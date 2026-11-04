package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestActivity(t *testing.T) {
	now := time.UnixMilli(1_000_000)
	var act Activity
	act.Touch(now)
	h := act.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), func() time.Time { return now })
	now = now.Add(20 * time.Minute)
	if got := act.Since(now); got != 20*time.Minute {
		t.Fatalf("since = %v", got)
	}
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	now = now.Add(5 * time.Minute)
	if got := act.Since(now); got != 5*time.Minute {
		t.Fatalf("since after a request = %v, want 5m", got)
	}
}

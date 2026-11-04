package web

import (
	"net/http"
	"sync/atomic"
	"time"
)

// Activity records when the server last got a request, so it can stop
// itself after a quiet spell. Safe for concurrent use.
type Activity struct{ last atomic.Int64 }

// Touch records a request at now.
func (a *Activity) Touch(now time.Time) { a.last.Store(now.UnixMilli()) }

// Since is how long before now the last request came.
func (a *Activity) Since(now time.Time) time.Duration {
	return time.Duration(now.UnixMilli()-a.last.Load()) * time.Millisecond
}

// Wrap returns h recording each request it serves.
func (a *Activity) Wrap(h http.Handler, now func() time.Time) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.Touch(now())
		h.ServeHTTP(w, r)
	})
}

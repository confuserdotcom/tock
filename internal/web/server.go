// Package web serves tock's dashboard on the loopback interface: the
// embedded page in static/ and a JSON API over the app package. Every API
// request carries the per-run token in the X-Tock-Token header; the page
// gets it from the URL fragment, which browsers never send to the server.
package web

import (
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"strconv"
	"sync"
	"time"

	"tock/internal/app"
)

//go:embed static
var staticFS embed.FS

// TokenHeader carries the per-run token on every API request.
const TokenHeader = "X-Tock-Token"

// maxBody is the largest POST body accepted.
const maxBody = 4 << 10

const csp = "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data:; " +
	"connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'"

// Server answers the dashboard's requests. One request at a time uses the
// app, which is not safe for concurrent use.
type Server struct {
	mu    sync.Mutex
	a     *app.App
	token string
	hosts map[string]bool
	now   func() time.Time
}

// NewToken returns a random 256-bit token as hex.
func NewToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// New returns a server for the app listening on port of the loopback
// interface. Requests naming any other host are refused, which defeats DNS
// rebinding.
func New(a *app.App, token string, port int, now func() time.Time) *Server {
	p := strconv.Itoa(port)
	return &Server{a: a, token: token, now: now, hosts: map[string]bool{
		"127.0.0.1:" + p: true, "localhost:" + p: true, "[::1]:" + p: true,
	}}
}

// Handler routes the page and the API.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	sub, _ := fs.Sub(staticFS, "static")
	files := http.FileServerFS(sub)
	mux.Handle("GET /", files)
	mux.HandleFunc("GET /api/day", s.api(s.day))
	mux.HandleFunc("GET /api/week", s.api(s.week))
	mux.HandleFunc("GET /api/top", s.api(s.top))
	mux.HandleFunc("GET /api/inbox", s.api(s.inbox))
	mux.HandleFunc("GET /api/labels", s.api(s.labels))
	mux.HandleFunc("GET /api/propose", s.api(s.propose))
	mux.HandleFunc("POST /api/tag", s.api(s.tag))
	mux.HandleFunc("POST /api/learn", s.api(s.learn))
	mux.HandleFunc("POST /api/ignore", s.api(s.ignore))
	mux.HandleFunc("POST /api/edit", s.api(s.edit))
	mux.HandleFunc("POST /api/undo", s.api(s.undo))
	return s.guard(mux)
}

// guard checks the host and sets the headers every response carries.
func (s *Server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.hosts[r.Host] {
			http.Error(w, "wrong host", http.StatusMisdirectedRequest)
			return
		}
		h := w.Header()
		h.Set("Content-Security-Policy", csp)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Cross-Origin-Resource-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

// httpError is an error with the status it answers with.
type httpError struct {
	code int
	msg  string
}

func (e httpError) Error() string { return e.msg }

func badRequest(msg string) error { return httpError{http.StatusBadRequest, msg} }

// api wraps a JSON handler: token check, body limits, one request at a
// time, rule reload, and errors as {"error": "..."}.
func (s *Server) api(fn func(*http.Request) (any, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if subtle.ConstantTimeCompare([]byte(r.Header.Get(TokenHeader)), []byte(s.token)) != 1 {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing or wrong token: open the link tock web printed"})
			return
		}
		if r.Method == http.MethodPost {
			if r.Header.Get("Content-Type") != "application/json" {
				writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "send application/json"})
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, maxBody)
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if _, err := s.a.Reload(); err != nil {
			// The last good rules stay in use; the page shows the problem.
			w.Header().Set("X-Tock-Rules-Error", "1")
		}
		v, err := fn(r)
		if err != nil {
			code := http.StatusInternalServerError
			var he httpError
			if errors.As(err, &he) {
				code = he.code
			} else if errors.Is(err, app.ErrRemote) {
				code = http.StatusConflict
			}
			writeJSON(w, code, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, v)
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// decode reads a JSON body into v, rejecting unknown fields.
func decode(r *http.Request, v any) error {
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return badRequest("bad request body")
	}
	return nil
}

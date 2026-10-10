//go:build demo

package web

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"testing"

	"tock/internal/app"
	"tock/internal/apptest"
)

// TestDemo serves synthetic data for working on the page:
//
//	TOCK_DEMO_PORT=7788 go test -tags demo -run TestDemo -timeout 0 ./internal/web
//
// then open http://127.0.0.1:7788/#<64 zeros>.
func TestDemo(t *testing.T) {
	e := apptest.New(t, apptest.Rules+`
[[goal]]
category = "Code"
at_least = "3h"

[[goal]]
category = "Entertainment"
at_most = "1h"
`)
	e.Seed(t, apptest.DaySpans()...)
	id := int64(100)
	add := func(day int, from, to, bundle, appName, title, url string) {
		id++
		e.Seed(t, apptest.Span{ID: id, Start: from, End: to, Bundle: bundle, App: appName, Title: title, URL: url, Day: day})
	}
	for d := -5; d <= -1; d++ {
		add(d, "08:30", "10:10", "term", "WezTerm", "tock │ make test", "")
		add(d, "10:10", "10:40", "arc", "Arc", "Video", "www.youtube.com/watch")
		add(d, "10:40", "12:00", "term", "WezTerm", "ATLAS │ notes", "")
		add(d, "12:00", "12:45", "tock.idle", "Idle", "", "")
		add(d, "12:45", "13:20", "mail", "Mail", "Inbox", "")
		add(d, "13:20", fmt.Sprintf("%02d:%02d", 14-d%2, 30+5*d), "arc", "Arc", "Docs", "docs.example.org/guide")
	}
	a, err := app.Open(app.Paths{DB: e.DB, Rules: e.Rules, Learned: e.Learned, Config: e.Config, ShellLog: e.ShellLog}, e.NowFunc())
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	port := os.Getenv("TOCK_DEMO_PORT")
	if port == "" {
		port = "7788"
	}
	ln, err := net.Listen("tcp", "127.0.0.1:"+port)
	if err != nil {
		t.Fatal(err)
	}
	token := fmt.Sprintf("%064d", 0)
	fmt.Printf("demo: http://127.0.0.1:%d/#%s\n", ln.Addr().(*net.TCPAddr).Port, token)
	t.Fatal(http.Serve(ln, New(a, token, ln.Addr().(*net.TCPAddr).Port, e.NowFunc()).Handler()))
}

package collect

import (
	"testing"

	"tock/internal/fronttab"
	"tock/internal/span"
)

func TestApplyFrontTab(t *testing.T) {
	sample := func(class, title string) *span.Sample {
		s := &span.Sample{}
		setWindow(s, class, title)
		return s
	}
	const since = 10_000
	cases := []struct {
		name, class, title string
		st                 fronttab.State
		url                string
		private            bool
	}{
		{"match", "chromium", "Play - Chromium", fronttab.State{AtMS: 1, Focused: true, URL: "chess.com/play", Title: "Play"}, "chess.com/play", false},
		{"other title (race)", "chromium", "Inbox - Chromium", fronttab.State{AtMS: 1, Focused: true, URL: "chess.com/play", Title: "Play"}, "", false},
		{"empty tab title", "chromium", "Chromium", fronttab.State{AtMS: 1, Focused: true, URL: "a.test"}, "", false},
		{"not a browser", "kitty", "Play - Chromium", fronttab.State{AtMS: 1, Focused: true, URL: "chess.com/play", Title: "Play"}, "", false},
		{"firefox", "firefox", "Play — Mozilla Firefox", fronttab.State{AtMS: 1, Focused: true, URL: "chess.com/play", Title: "Play"}, "chess.com/play", false},
		{"incognito, fresh", "chromium", "Secret - Chromium", fronttab.State{AtMS: since - 500, Focused: true, Private: true}, "", true},
		{"incognito, old", "chromium", "Secret - Chromium", fronttab.State{AtMS: 1, Focused: true, Private: true}, "", false},
		{"no normal window, fresh", "chromium", "Secret - Chromium", fronttab.State{AtMS: since + 50}, "", true},
		{"no normal window, old", "chromium", "Docs - Chromium", fronttab.State{AtMS: 1}, "", false},
	}
	for _, c := range cases {
		s := sample(c.class, c.title)
		applyFrontTab(s, c.st, since)
		got := ""
		if s.URL != nil {
			got = *s.URL
		}
		if got != c.url || s.Private != c.private || (c.private && s.Title != nil) {
			t.Errorf("%s: url %q private %v title %v", c.name, got, s.Private, s.Title)
		}
	}
}

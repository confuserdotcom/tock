package collect

import "testing"

func TestCloseIfHost(t *testing.T) {
	var ran []browserKind
	run := func(url string) scriptRunner {
		return func(k browserKind, warm bool) (string, int, bool) {
			ran = append(ran, k)
			if k == 21 || k == 24 {
				return "ok", 0, true
			}
			return url, 0, true
		}
	}
	cases := []struct {
		bundle, host, url string
		closed            bool
		ran               []browserKind
	}{
		{"com.google.Chrome", "www.youtube.com", "https://www.youtube.com/watch?v=1", true, []browserKind{1, 21}},
		{"com.apple.Safari", "x.com", "https://x.com:443/home", true, []browserKind{4, 24}},
		{"com.google.Chrome", "www.youtube.com", "https://github.com/", false, []browserKind{1}},
		{"com.google.Chrome", "www.youtube.com", "private", false, []browserKind{1}},
		{"org.mozilla.firefox", "www.youtube.com", "", false, nil},
		{"com.google.Chrome", "", "", false, nil},
	}
	for _, c := range cases {
		ran = nil
		closed, err := closeIfHost(c.bundle, c.host, run(c.url))
		if err != nil || closed != c.closed || len(ran) != len(c.ran) {
			t.Errorf("closeIfHost(%s, %s, %s) = %v, %v; ran %v", c.bundle, c.host, c.url, closed, err, ran)
			continue
		}
		for i := range ran {
			if ran[i] != c.ran[i] {
				t.Errorf("%s: ran %v, want %v", c.url, ran, c.ran)
			}
		}
	}
	fail := func(browserKind, bool) (string, int, bool) { return "", -1743, false }
	if _, err := closeIfHost("com.google.Chrome", "a.com", fail); err == nil {
		t.Error("script failure: nil error")
	}
}

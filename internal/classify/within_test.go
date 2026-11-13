package classify

import "testing"

func TestWithin(t *testing.T) {
	cases := []struct {
		dir, root  string
		fold, want bool
	}{
		{"/r/tock/web", "/r/tock", false, true},
		{"/r/tock", "/r/tock/", false, true},
		{"/r/tockx", "/r/tock", false, false},
		{"/R/Tock/web", "/r/tock", false, false},
		{"/R/Tock/web", "/r/tock", true, true},
		{"/", "/", false, true},
		{"/x", "/", false, true},
		{"", "/r", false, false},
	}
	defer func(f bool) { foldPaths = f }(foldPaths)
	for _, c := range cases {
		foldPaths = c.fold
		if got := Within(c.dir, c.root); got != c.want {
			t.Errorf("Within(%q, %q) fold=%v = %v", c.dir, c.root, c.fold, got)
		}
	}
}

func TestExpandHomeBackslashOnlyWithBackslashSeparator(t *testing.T) {
	if got := ExpandHome("~/x", "/h"); got != "/h/x" {
		t.Fatalf("~/x = %q", got)
	}
	if got := ExpandHome("~", "/h"); got != "/h" {
		t.Fatalf("~ = %q", got)
	}
}

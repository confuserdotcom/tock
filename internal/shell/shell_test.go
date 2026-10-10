package shell

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"tock/internal/classify"
)

func write(t *testing.T, path, s string, flag int) {
	t.Helper()
	f, err := os.OpenFile(path, flag|os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(s); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestLogEvents(t *testing.T) {
	path := filepath.Join(t.TempDir(), File)
	l := &Log{Path: path}
	if ev, err := l.Events(); err != nil || ev != nil {
		t.Fatalf("missing file: %v %v", ev, err)
	}

	write(t, path, "1000\t/a\nbad\n2000\t/b\n\n3000\tx\nx\t/c\n4000\t/par", os.O_TRUNC)
	ev, err := l.Events()
	if err != nil {
		t.Fatal(err)
	}
	if want := []Event{{1000, "/a"}, {2000, "/b"}}; !reflect.DeepEqual(ev, want) {
		t.Fatalf("got %v, want %v", ev, want)
	}

	// the partial line completes; an out-of-order line gets sorted in
	write(t, path, "tial\n1500\t/d\n", os.O_APPEND)
	ev, _ = l.Events()
	if want := []Event{{1000, "/a"}, {1500, "/d"}, {2000, "/b"}, {4000, "/partial"}}; !reflect.DeepEqual(ev, want) {
		t.Fatalf("after append: got %v, want %v", ev, want)
	}

	write(t, path, "9\t/z\n", os.O_TRUNC)
	ev, _ = l.Events()
	if want := []Event{{9, "/z"}}; !reflect.DeepEqual(ev, want) {
		t.Fatalf("after shrink: got %v, want %v", ev, want)
	}

	os.Remove(path)
	if ev, err := l.Events(); err != nil || ev != nil {
		t.Fatalf("removed file: %v %v", ev, err)
	}
}

func dir(v classify.SpanView) string {
	if v.Dir == nil {
		return "<nil>"
	}
	return *v.Dir
}

func TestAttach(t *testing.T) {
	term := []string{"term"}
	for _, c := range []struct {
		name   string
		bundle string
		ev     []Event
		home   string
		want   string
	}{
		{"majority", "term", []Event{{10, "/a"}, {30, "/b"}}, "", "/b"},
		{"event before start", "term", []Event{{-5, "/a"}}, "", "/a"},
		{"older events ignored", "term", []Event{{-9, "/old"}, {-5, "/a"}}, "", "/a"},
		{"all after end", "term", []Event{{100, "/a"}}, "", "<nil>"},
		{"not a terminal", "arc", []Event{{0, "/a"}}, "", "<nil>"},
		{"tie goes to the first", "term", []Event{{0, "/a"}, {50, "/b"}}, "", "/a"},
		{"split time adds up", "term", []Event{{0, "/a"}, {20, "/b"}, {40, "/a"}, {60, "/b"}, {70, "/c"}}, "", "/a"},
		{"home", "term", []Event{{0, "/Users/u"}}, "/Users/u", "~"},
		{"under home", "term", []Event{{0, "/Users/u/code"}}, "/Users/u/", "~/code"},
		{"sibling of home", "term", []Event{{0, "/Users/uu"}}, "/Users/u", "/Users/uu"},
	} {
		vs := []classify.SpanView{{ID: 1, Start: 0, End: 100, BundleID: c.bundle}}
		Attach(vs, c.ev, term, c.home)
		if got := dir(vs[0]); got != c.want {
			t.Errorf("%s: dir = %s, want %s", c.name, got, c.want)
		}
	}
}

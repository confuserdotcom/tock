package classify

import (
	"bufio"
	"os"
	"strings"
	"testing"
)

// TestStarterCorpus checks the starter rules against synthetic samples of the
// apps and sites they promise to cover (phase 2 success criterion 2).
func TestStarterCorpus(t *testing.T) {
	rs, err := Parse(Starter, nil)
	if err != nil {
		t.Fatalf("starter rules invalid: %v", err)
	}
	f, err := os.Open("testdata/starter_corpus.tsv")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	n := 0
	for line := 1; sc.Scan(); line++ {
		if strings.HasPrefix(sc.Text(), "#") || sc.Text() == "" {
			continue
		}
		c := strings.Split(sc.Text(), "\t")
		if len(c) != 6 {
			t.Fatalf("line %d: %d columns, want 6", line, len(c))
		}
		v := SpanView{BundleID: c[0], App: c[1], Title: optField(c[2]), URL: optField(c[3])}
		l := rs.Label(v, nil)
		if dash(l.Category) != c[4] || dash(l.Project) != c[5] {
			t.Errorf("line %d (%s %q %s): got %s / %s, want %s / %s",
				line, c[1], c[2], c[3], dash(l.Category), dash(l.Project), c[4], c[5])
		}
		n++
	}
	if n < 60 {
		t.Fatalf("corpus has %d samples, want at least 60", n)
	}
}

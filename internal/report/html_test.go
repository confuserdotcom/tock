package report

import (
	"bytes"
	"strings"
	"testing"
)

func TestWeekHTMLEscapes(t *testing.T) {
	wk := Week{Monday: MondayOf(day0), TrackedMS: 3_600_000, Plan: `<script>x</script>`,
		Categories: []CatRow{{Name: `<b>Code</b>`, Kind: "deep", MS: 3_600_000}},
		Apps:       []NameMS{{"evil.com/<img>", 60_000}}}
	var b bytes.Buffer
	if err := WriteWeekHTML(&b, wk, day0); err != nil {
		t.Fatal(err)
	}
	s := b.String()
	if strings.Contains(s, "<script>") || strings.Contains(s, "<b>Code") || strings.Contains(s, "<img>") {
		t.Fatalf("unescaped input in page")
	}
	if !strings.Contains(s, "&lt;b&gt;Code&lt;/b&gt;") || !strings.Contains(s, "<b>—</b><span>average score</span>") {
		t.Errorf("page:\n%s", s)
	}
}

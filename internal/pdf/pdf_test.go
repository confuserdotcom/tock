package pdf

import (
	"bytes"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestDocumentStructure(t *testing.T) {
	var d Doc
	p := d.AddPage()
	p.Text(50, 60, 12, true, "Invoice (draft) \\ 1")
	p.TextRight(545, 60, 10, false, "1 234,50 €")
	p.Line(50, 70, 545, 70, 0.5, 0.6)
	p.Rect(50, 80, 100, 20, 0.95)
	d.AddPage().Text(50, 60, 10, false, "page 2")
	var b bytes.Buffer
	if _, err := d.WriteTo(&b); err != nil {
		t.Fatal(err)
	}
	s := b.String()
	if !strings.HasPrefix(s, "%PDF-1.4\n") || !strings.HasSuffix(s, "%%EOF\n") {
		t.Fatal("header or trailer missing")
	}
	if !strings.Contains(s, `(Invoice \(draft\) \\ 1) Tj`) || !strings.Contains(s, "/Count 2") {
		t.Errorf("text or page count wrong:\n%s", s)
	}
	if !bytes.Contains(b.Bytes(), []byte("(1 234,50 \x80) Tj")) {
		t.Error("euro sign not WinAnsi encoded")
	}
	// Every xref offset points at its "N 0 obj".
	m := regexp.MustCompile(`startxref\n(\d+)\n`).FindStringSubmatch(s)
	at, _ := strconv.Atoi(m[1])
	lines := strings.Split(s[at:], "\n")
	count, _ := strconv.Atoi(strings.Fields(lines[1])[1])
	for i := 1; i < count; i++ {
		off, _ := strconv.Atoi(strings.Fields(lines[2+i])[0])
		if want := strconv.Itoa(i) + " 0 obj"; !strings.HasPrefix(s[off:], want) {
			t.Fatalf("xref %d points at %q", i, s[off:off+10])
		}
	}
	var again bytes.Buffer
	d.WriteTo(&again)
	if !bytes.Equal(b.Bytes(), again.Bytes()) {
		t.Error("output not deterministic")
	}
}

func TestEncodeAndWidth(t *testing.T) {
	if got := string(Encode("é–日\n")); got != "\xe9\x96? " {
		t.Errorf("encode %q", got)
	}
	if w := Width("Hi", 10, false); w != (722+222)*10.0/1000 {
		t.Errorf("width %v", w)
	}
	if Width("W", 10, true) <= Width("i", 10, true) {
		t.Error("bold widths not used")
	}
}

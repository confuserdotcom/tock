// Package pdf writes simple PDF documents with no dependencies: A4 pages
// of Helvetica text, lines and filled rectangles. Text is WinAnsi encoded;
// characters it cannot hold become "?". Coordinates are in points from the
// page's top-left corner.
package pdf

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// A4 page size in points.
const (
	PageW = 595.28
	PageH = 841.89
)

// Doc is a document being built.
type Doc struct{ pages []*Page }

// Page is one page's drawing operations.
type Page struct{ ops bytes.Buffer }

// AddPage appends a blank page and returns it.
func (d *Doc) AddPage() *Page {
	p := &Page{}
	d.pages = append(d.pages, p)
	return p
}

// Pages is the number of pages.
func (d *Doc) Pages() int { return len(d.pages) }

func num(v float64) string {
	s := strconv.FormatFloat(v, 'f', 2, 64)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	if s == "" || s == "-0" {
		return "0"
	}
	return s
}

// Text draws s with its baseline at y, starting at x.
func (p *Page) Text(x, y, size float64, bold bool, s string) {
	font := "F1"
	if bold {
		font = "F2"
	}
	fmt.Fprintf(&p.ops, "BT /%s %s Tf %s %s Td (%s) Tj ET\n", font, num(size), num(x), num(PageH-y), escape(Encode(s)))
}

// TextRight draws s ending at x.
func (p *Page) TextRight(x, y, size float64, bold bool, s string) {
	p.Text(x-Width(s, size, bold), y, size, bold, s)
}

// Line draws a line of the given width and gray level (0 black, 1 white).
func (p *Page) Line(x1, y1, x2, y2, width, gray float64) {
	fmt.Fprintf(&p.ops, "q %s G %s w %s %s m %s %s l S Q\n", num(gray), num(width),
		num(x1), num(PageH-y1), num(x2), num(PageH-y2))
}

// Rect fills a rectangle whose top-left corner is (x, y) with a gray level.
func (p *Page) Rect(x, y, w, h, gray float64) {
	fmt.Fprintf(&p.ops, "q %s g %s %s %s %s re f Q\n", num(gray), num(x), num(PageH-y-h), num(w), num(h))
}

// winAnsi maps the non-Latin-1 characters WinAnsiEncoding holds.
var winAnsi = map[rune]byte{
	'€': 0x80, '‚': 0x82, 'ƒ': 0x83, '„': 0x84, '…': 0x85, '†': 0x86, '‡': 0x87, 'ˆ': 0x88, '‰': 0x89,
	'Š': 0x8A, '‹': 0x8B, 'Œ': 0x8C, 'Ž': 0x8E, '‘': 0x91, '’': 0x92, '“': 0x93, '”': 0x94, '•': 0x95,
	'–': 0x96, '—': 0x97, '˜': 0x98, '™': 0x99, 'š': 0x9A, '›': 0x9B, 'œ': 0x9C, 'ž': 0x9E, 'Ÿ': 0x9F,
}

// Encode turns s into WinAnsi bytes; anything else becomes '?', and
// control characters become spaces.
func Encode(s string) []byte {
	out := make([]byte, 0, len(s))
	for _, r := range s {
		switch {
		case r < 0x20 || r == 0x7F:
			out = append(out, ' ')
		case r < 0x7F, r >= 0xA0 && r <= 0xFF:
			out = append(out, byte(r))
		default:
			if b, ok := winAnsi[r]; ok {
				out = append(out, b)
			} else {
				out = append(out, '?')
			}
		}
	}
	return out
}

func escape(b []byte) string {
	var sb strings.Builder
	for _, c := range b {
		switch c {
		case '(', ')', '\\':
			sb.WriteByte('\\')
			sb.WriteByte(c)
		default:
			sb.WriteByte(c)
		}
	}
	return sb.String()
}

// Helvetica and Helvetica-Bold advance widths of the characters 32 to 126,
// in thousandths of the font size (from the Adobe AFM files).
var widths = [2][95]int{
	{278, 278, 355, 556, 556, 889, 667, 191, 333, 333, 389, 584, 278, 333, 278, 278,
		556, 556, 556, 556, 556, 556, 556, 556, 556, 556, 278, 278, 584, 584, 584, 556,
		1015, 667, 667, 722, 722, 667, 611, 778, 722, 278, 500, 667, 556, 833, 722, 778,
		667, 778, 722, 667, 611, 722, 667, 944, 667, 667, 611, 278, 278, 278, 469, 556,
		333, 556, 556, 500, 556, 556, 278, 556, 556, 222, 222, 500, 222, 833, 556, 556,
		556, 556, 333, 500, 278, 556, 500, 722, 500, 500, 500, 334, 260, 334, 584},
	{278, 333, 474, 556, 556, 889, 722, 238, 333, 333, 389, 584, 278, 333, 278, 278,
		556, 556, 556, 556, 556, 556, 556, 556, 556, 556, 333, 333, 584, 584, 584, 611,
		975, 722, 722, 722, 722, 667, 611, 778, 722, 278, 556, 722, 611, 833, 722, 778,
		667, 778, 722, 667, 611, 722, 667, 944, 667, 667, 611, 333, 278, 333, 584, 556,
		333, 556, 611, 556, 611, 556, 333, 611, 611, 278, 278, 556, 278, 889, 611, 611,
		611, 611, 389, 556, 333, 611, 556, 778, 556, 556, 500, 389, 280, 389, 584},
}

// Width is the width of s in points at size. Characters outside ASCII
// count as 556, an average.
func Width(s string, size float64, bold bool) float64 {
	t := 0
	if bold {
		t = 1
	}
	total := 0
	for _, c := range Encode(s) {
		if c >= 32 && c <= 126 {
			total += widths[t][c-32]
		} else {
			total += 556
		}
	}
	return float64(total) * size / 1000
}

// WriteTo writes the document. It holds no dates, so the same document
// always gives the same bytes.
func (d *Doc) WriteTo(w io.Writer) (int64, error) {
	var b bytes.Buffer
	var offsets []int
	obj := func(body string) {
		offsets = append(offsets, b.Len())
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", len(offsets), body)
	}
	b.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")
	kids := make([]string, len(d.pages))
	for i := range d.pages {
		kids[i] = fmt.Sprintf("%d 0 R", 5+2*i)
	}
	obj("<< /Type /Catalog /Pages 2 0 R >>")
	obj(fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(d.pages)))
	obj("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>")
	obj("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold /Encoding /WinAnsiEncoding >>")
	for i, p := range d.pages {
		obj(fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %s %s] /Resources << /Font << /F1 3 0 R /F2 4 0 R >> >> /Contents %d 0 R >>",
			num(PageW), num(PageH), 6+2*i))
		obj(fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", p.ops.Len(), p.ops.String()))
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(offsets)+1)
	for _, o := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets)+1, xref)
	n, err := w.Write(b.Bytes())
	return int64(n), err
}

package tui

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Zones make anything on screen clickable. Rendering wraps a piece of text
// in z(id, …): invisible APC markers that take no cells and survive
// ansi.Truncate. render() then finds them, strips them and records where
// each zone landed, so a click (or the mouse hovering) maps back to an id.

const (
	zOpen  = "\x1b_tz:"
	zClose = "\x1b_tz\x1b\\"
	zST    = "\x1b\\"
)

// hover is the id of the zone under the mouse while a frame renders; z
// underlines it. render() sets it from Model.zones.
var hover string

// zoneIDs are the ids of the frame's zones: a marker carries only an index
// into it, since a terminal parser may end an APC string at a byte of a
// multi-byte rune.
var zoneIDs []string

// z marks s as zone id.
func z(id, s string) string {
	if id == "" || s == "" {
		return s
	}
	key := strconv.Itoa(len(zoneIDs))
	zoneIDs = append(zoneIDs, id)
	if id == hover {
		if strings.HasPrefix(id, "row:") || strings.HasPrefix(id, "opt:") || strings.HasPrefix(id, "menu:") {
			s = tint(s)
		} else {
			s = underline(s)
		}
	}
	return zOpen + key + zST + s + zClose
}

// tint puts a hovered row on the hover background, through its resets.
func tint(s string) string {
	probe := sHover.Render("x")
	bg, _, _ := strings.Cut(probe, "x")
	if bg == "" {
		return s
	}
	s = strings.ReplaceAll(s, "\x1b[0m", "\x1b[0m"+bg)
	s = strings.ReplaceAll(s, "\x1b[m", "\x1b[m"+bg)
	return bg + s + "\x1b[49m"
}

// underline underlines s through its own colour resets.
func underline(s string) string {
	const on = "\x1b[4m"
	s = strings.ReplaceAll(s, "\x1b[0m", "\x1b[0m"+on)
	s = strings.ReplaceAll(s, "\x1b[m", "\x1b[m"+on)
	return on + s + "\x1b[24m"
}

// hit is where one zone landed: row y, cells [x0, x1).
type hit struct {
	id        string
	y, x0, x1 int
}

// zoneMap is the last frame's zones and the mouse's position over them.
// The model holds a pointer so View, a value method, can fill it.
type zoneMap struct {
	hits  []hit
	tips  map[string]string // tooltip by zone id, shown in the footer on hover
	hover string
}

// at is the innermost zone covering cell (x, y).
func (zm *zoneMap) at(x, y int) (string, bool) {
	if zm == nil {
		return "", false
	}
	best, bestW := "", -1
	for _, h := range zm.hits {
		if h.y == y && x >= h.x0 && x < h.x1 && (bestW < 0 || h.x1-h.x0 < bestW) {
			best, bestW = h.id, h.x1-h.x0
		}
	}
	return best, bestW >= 0
}

// tipBuf collects tooltips while a frame renders.
var tipBuf map[string]string

// tip gives zone id a tooltip and returns z(id, s).
func tip(id, text, s string) string {
	if tipBuf != nil && text != "" {
		tipBuf[id] = text
	}
	return z(id, s)
}

// scanZones strips the zone markers from lines and returns where each
// zone is. A zone left open at the end of a line (cut by fitting) closes
// there; markers nest.
func scanZones(lines []string) ([]string, []hit) {
	name := func(key string) string {
		if i, err := strconv.Atoi(key); err == nil && i >= 0 && i < len(zoneIDs) {
			return zoneIDs[i]
		}
		return key
	}
	var hits []hit
	out := make([]string, len(lines))
	for y, l := range lines {
		if !strings.Contains(l, zOpen) {
			out[y] = l
			continue
		}
		var b strings.Builder
		type open struct {
			id string
			x  int
		}
		var stack []open
		col := 0
		for l != "" {
			i := strings.Index(l, "\x1b_tz")
			if i < 0 {
				b.WriteString(l)
				col += ansi.StringWidth(l)
				break
			}
			b.WriteString(l[:i])
			col += ansi.StringWidth(l[:i])
			l = l[i:]
			switch {
			case strings.HasPrefix(l, zClose):
				l = l[len(zClose):]
				if n := len(stack); n > 0 {
					o := stack[n-1]
					stack = stack[:n-1]
					if col > o.x {
						hits = append(hits, hit{o.id, y, o.x, col})
					}
				}
			case strings.HasPrefix(l, zOpen):
				end := strings.Index(l, zST)
				if end < 0 {
					l = ""
					break
				}
				stack = append(stack, open{name(l[len(zOpen):end]), col})
				l = l[end+len(zST):]
			default: // not ours: keep it
				b.WriteString(l[:4])
				l = l[4:]
			}
		}
		for _, o := range stack {
			if col > o.x {
				hits = append(hits, hit{o.id, y, o.x, col})
			}
		}
		out[y] = b.String()
	}
	return out, hits
}

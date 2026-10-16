package tui

import (
	"reflect"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestScanZones(t *testing.T) {
	zoneIDs = nil
	lines := []string{
		"ab" + z("x", sBrand.Render("cd")) + "ef",
		z("row", "12 "+z("k", "t tag")+" 34"),
		"plain",
		ansi.Truncate("é "+z("cut", "long text here"), 6, ""),
		z("cat:—", "ab"),
	}
	clean, hits := scanZones(lines)
	if ansi.Strip(clean[0]) != "abcdef" || clean[2] != "plain" || ansi.Strip(clean[1]) != "12 t tag 34" {
		t.Fatalf("clean = %q", clean)
	}
	want := []hit{{"x", 0, 2, 4}, {"k", 1, 3, 8}, {"row", 1, 0, 11}, {"cut", 3, 2, 6}, {"cat:—", 4, 0, 2}}
	if !reflect.DeepEqual(hits, want) {
		t.Fatalf("hits = %+v", hits)
	}
	zm := &zoneMap{hits: hits}
	if id, ok := zm.at(4, 1); !ok || id != "k" {
		t.Fatalf("innermost = %q", id)
	}
	if id, _ := zm.at(1, 1); id != "row" {
		t.Fatalf("outer = %q", id)
	}
	if _, ok := zm.at(9, 0); ok {
		t.Fatal("hit past the zone")
	}
}

func TestZoneHoverUnderlines(t *testing.T) {
	hover = "x"
	defer func() { hover = "" }()
	zoneIDs = nil
	if s := z("x", "a"); s != zOpen+"0"+zST+"\x1b[4ma\x1b[24m"+zClose || zoneIDs[0] != "x" {
		t.Fatalf("hovered = %q", s)
	}
}

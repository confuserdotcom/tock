package config

import (
	"strings"
	"testing"
)

// A quoted value with spaces is replaced whole, keeping the comment after it.
func TestSetStringKeepsComment(t *testing.T) {
	src := "[invoice]\nname = \"Ada Lovelace\" # me\ncurrency = \"EUR\"\n"
	got, err := SetString([]byte(src), "invoice", "name", "Grace \"Amazing\" Hopper")
	if err != nil {
		t.Fatal(err)
	}
	want := "[invoice]\nname = \"Grace \\\"Amazing\\\" Hopper\" # me\ncurrency = \"EUR\"\n"
	if string(got) != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
	got, err = SetString([]byte("[x]\ns = 'a b' # c\n"), "x", "s", "z")
	if err != nil || string(got) != "[x]\ns = \"z\" # c\n" {
		t.Fatalf("literal string: %q %v", got, err)
	}
}

func TestSetIntAndInts(t *testing.T) {
	got, err := SetInts([]byte("[focus]\npresets = [25, 50] # mine\n"), "focus", "presets", []int{45, 90})
	if err != nil || string(got) != "[focus]\npresets = [45, 90] # mine\n" {
		t.Fatalf("ints: %q %v", got, err)
	}
	got, err = SetInt(nil, "invoice", "minor_units", 0)
	if err != nil || string(got) != "[invoice]\nminor_units = 0\n" {
		t.Fatalf("int: %q %v", got, err)
	}
}

// SaveValues writes several keys at once; nil unsets one; a result that
// does not load writes nothing and says why.
func TestSaveValues(t *testing.T) {
	path := t.TempDir() + "/" + File
	src := "# my config\n[hours]\nstart = \"09:00\"\nend = \"17:00\"\nover_grace = 30\n"
	writeFile(t, path, src)
	err := SaveValues(path, []Change{
		{"hours", "start", "08:30"}, {"hours", "days", []string{"mon", "tue"}},
		{"focus", "presets", []int{50, 25}}, {"invoice", "name", "Ada"}, {"invoice", "minor_units", 0},
	})
	if err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil || c.Hours.StartMin != 8*60+30 || !c.Hours.Days[1] || c.Hours.Days[3] || c.Focus.Presets[0] != 50 || c.Invoice.Name != "Ada" || c.Invoice.MinorUnits != 0 {
		t.Fatalf("after save: %+v %+v %v", c.Hours, c.Invoice, err)
	}
	if !strings.HasPrefix(readFile(t, path), "# my config\n") {
		t.Fatal("comment lost")
	}
	// Hours off: start, end and days go, over_grace stays.
	if err := SaveValues(path, []Change{{"hours", "start", nil}, {"hours", "end", nil}, {"hours", "days", nil}}); err != nil {
		t.Fatal(err)
	}
	if c, _ := Load(path); c.Hours.Set || !strings.Contains(readFile(t, path), "over_grace = 30") {
		t.Fatalf("hours off: %+v\n%s", c.Hours, readFile(t, path))
	}
	before := readFile(t, path)
	err = SaveValues(path, []Change{{"hours", "start", "18:00"}, {"hours", "end", "09:00"}})
	if err == nil || !strings.Contains(err.Error(), "not before end") || readFile(t, path) != before {
		t.Fatalf("bad hours: %v", err)
	}
}

// The clients are rewritten as a block at the end; the rest stays.
func TestSetClients(t *testing.T) {
	src := "[invoice]\nname = \"Ada\"\n\n[[invoice.client]]\nname = \"Old\" # gone\n\n[score]\ndepth = 1.0\nblocks = 0\ncalm = 0\nstretch = 0\n"
	got, err := SetClients([]byte(src), []InvoiceClient{{Name: "Babbage & Co", Address: []string{"1 Engine St", "London"}, TaxID: "GB1"}, {Name: "Lamb"}})
	if err != nil {
		t.Fatal(err)
	}
	c, err := Parse(File, got)
	if err != nil {
		t.Fatalf("%v\n%s", err, got)
	}
	if len(c.Invoice.Clients) != 2 || c.Invoice.Clients[0].Address[1] != "London" || c.Invoice.Clients[1].Name != "Lamb" || c.Score.Depth != 1 {
		t.Fatalf("clients %+v\n%s", c.Invoice.Clients, got)
	}
	if strings.Contains(string(got), "Old") {
		t.Fatalf("old client kept:\n%s", got)
	}
	none, err := SetClients(got, nil)
	if err != nil || strings.Contains(string(none), "invoice.client") {
		t.Fatalf("no clients: %v\n%s", err, none)
	}
}

func TestFormatTaxRate(t *testing.T) {
	for bp, want := range map[int64]string{2000: "20", 550: "5.5", 1925: "19.25", 0: "0", 5: "0.05"} {
		if got := FormatTaxRate(bp); got != want {
			t.Errorf("%d: %q want %q", bp, got, want)
		}
		if back, err := ParseTaxRate(FormatTaxRate(bp)); err != nil || back != bp {
			t.Errorf("%d: round trip %d %v", bp, back, err)
		}
	}
}

package config

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadRoutineStart(t *testing.T) {
	c, err := Load(write(t, ""))
	if err != nil || !c.Routine.Start {
		t.Fatalf("empty file: Start = %v, err = %v, want true", c.Routine.Start, err)
	}
	m, err := Load(filepath.Join(t.TempDir(), File))
	if err != nil || !reflect.DeepEqual(m, Default()) {
		t.Fatalf("missing file: got %+v, %v, want Default()", m, err)
	}
	c, err = Load(write(t, "[routine]\nstart = false\n"))
	if err != nil || c.Routine.Start {
		t.Fatalf("start = false: Start = %v, err = %v", c.Routine.Start, err)
	}
	_, err = Load(write(t, "[routine]\nbogus = 1\n"))
	if err == nil || !strings.Contains(err.Error(), "unknown key routine.bogus") {
		t.Fatalf("unknown key: err = %v, want unknown key routine.bogus", err)
	}
}

func TestLoadRoutineMidday(t *testing.T) {
	c, err := Load(write(t, ""))
	if err != nil || c.Routine.MiddayAt != 780 {
		t.Fatalf("default: MiddayAt = %d, err = %v, want 780", c.Routine.MiddayAt, err)
	}
	for body, want := range map[string]int{"12:30": 750, "off": -1} {
		c, err := Load(write(t, "[routine]\nmidday = \""+body+"\"\n"))
		if err != nil || c.Routine.MiddayAt != want {
			t.Fatalf("midday = %q: MiddayAt = %d, err = %v, want %d", body, c.Routine.MiddayAt, err, want)
		}
	}
	for _, bad := range []string{"", "25:00", "1pm"} {
		_, err := Load(write(t, "[routine]\nmidday = \""+bad+"\"\n"))
		want := `[routine] midday = "` + bad + `", want "HH:MM" or "off"`
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("midday = %q: err = %v, want %s", bad, err, want)
		}
	}
}

func TestLoadRoutineEnd(t *testing.T) {
	if d := Default(); !d.Routine.End || d.Focus.RecapAt != 1260 {
		t.Fatalf("Default: End = %v, RecapAt = %d, want true, 1260", d.Routine.End, d.Focus.RecapAt)
	}
	for _, tt := range []struct {
		name, body string
		want       int
	}{
		{"end off", "[routine]\nend = false\n", -1},
		{"end off with hours", "[hours]\nstart = \"09:00\"\nend = \"18:00\"\n[routine]\nend = false\n", -1},
		{"end off with recap time", "[focus]\nrecap = \"20:00\"\n[routine]\nend = false\n", -1},
		{"hours recap default", "[hours]\nstart = \"09:00\"\nend = \"18:00\"\n", 1080},
		{"end on", "[routine]\nend = true\n", 1260},
	} {
		c, err := Load(write(t, tt.body))
		if err != nil || c.Focus.RecapAt != tt.want {
			t.Errorf("%s: RecapAt = %d, err = %v, want %d", tt.name, c.Focus.RecapAt, err, tt.want)
		}
	}
}

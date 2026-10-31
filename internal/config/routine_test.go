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

func TestLoadRoutineOverwork(t *testing.T) {
	r := Default().Routine
	if !r.Overwork || r.OverGraceMS != 1_800_000 || r.OverRepeatMS != 3_600_000 {
		t.Fatalf("Default: %+v", r)
	}
	c, err := Load(write(t, "[routine]\noverwork = false\n"))
	if err != nil || c.Routine.Overwork {
		t.Fatalf("overwork = false: Overwork = %v, err = %v", c.Routine.Overwork, err)
	}
	c, err = Load(write(t, "[routine]\noverwork = true\n"))
	if err != nil || !c.Routine.Overwork {
		t.Fatalf("overwork = true: Overwork = %v, err = %v", c.Routine.Overwork, err)
	}
}

func TestLoadHoursOver(t *testing.T) {
	c, err := Load(write(t, "[hours]\nstart = \"09:00\"\nend = \"18:00\"\nover_grace = 45\nover_repeat = 90\n"))
	if err != nil || c.Routine.OverGraceMS != 2_700_000 || c.Routine.OverRepeatMS != 5_400_000 {
		t.Fatalf("got %+v, %v", c.Routine, err)
	}
	for _, tt := range []struct{ body, want string }{
		{"over_grace = 4", "[hours] over_grace = 4, want 5 to 240"},
		{"over_grace = 241", "[hours] over_grace = 241, want 5 to 240"},
		{"over_repeat = 14", "[hours] over_repeat = 14, want 15 to 480"},
		{"over_repeat = 481", "[hours] over_repeat = 481, want 15 to 480"},
		{"over_soon = 1", "unknown key hours.over_soon"},
	} {
		_, err := Load(write(t, "[hours]\nstart = \"09:00\"\nend = \"18:00\"\n"+tt.body+"\n"))
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: err = %v, want %q", tt.body, err, tt.want)
		}
	}
	c, err = Load(write(t, "[hours]\nover_grace = 45\n"))
	if err != nil || c.Hours.Set || c.Routine.OverGraceMS != 2_700_000 || c.Routine.OverRepeatMS != 3_600_000 {
		t.Fatalf("over_grace alone: Hours.Set = %v, %+v, %v", c.Hours.Set, c.Routine, err)
	}
	c, err = Load(write(t, "[hours]\nover_repeat = 20\n"))
	if err != nil || c.Hours.Set || c.Routine.OverRepeatMS != 1_200_000 {
		t.Fatalf("over_repeat alone: Hours.Set = %v, %+v, %v", c.Hours.Set, c.Routine, err)
	}
}

// TestRoutineDocDefaults pins the values shown in the README to DefaultRoutine.
func TestRoutineDocDefaults(t *testing.T) {
	doc := "[hours]\nstart = \"09:00\"\nend = \"18:00\"\nover_grace = 30\nover_repeat = 60\n" +
		"[routine]\nstart = true\nmidday = \"13:00\"\nend = true\noverwork = true\n"
	c, err := Load(write(t, doc))
	if err != nil {
		t.Fatalf("documented config: %v", err)
	}
	if !reflect.DeepEqual(c.Routine, DefaultRoutine()) {
		t.Fatalf("documented values = %+v, want DefaultRoutine() %+v", c.Routine, DefaultRoutine())
	}
}

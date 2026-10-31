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

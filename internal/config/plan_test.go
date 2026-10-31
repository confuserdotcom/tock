package config

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadPlanNotices(t *testing.T) {
	c, err := Load(write(t, ""))
	if err != nil || !c.Plan.Notify || c.Plan.NotifyBeforeMS != 300_000 {
		t.Fatalf("empty file: %+v, err %v, want notify on, 5 min", c.Plan, err)
	}
	m, err := Load(filepath.Join(t.TempDir(), File))
	if err != nil || !reflect.DeepEqual(m, Default()) || !m.Plan.Notify {
		t.Fatalf("missing file: got %+v, %v, want Default()", m, err)
	}
	c, err = Load(write(t, "[plan]\nnotify = false\n"))
	if err != nil || c.Plan.Notify || c.Plan.NotifyBeforeMS != 300_000 {
		t.Fatalf("notify = false: %+v, err %v", c.Plan, err)
	}
	for _, tt := range []struct {
		body string
		want int64
	}{{"0", 0}, {"60", 3_600_000}, {"15", 900_000}} {
		c, err = Load(write(t, "[plan]\nnotify_before = "+tt.body+"\n"))
		if err != nil || c.Plan.NotifyBeforeMS != tt.want {
			t.Fatalf("notify_before = %s: %d, err %v, want %d", tt.body, c.Plan.NotifyBeforeMS, err, tt.want)
		}
	}
	for _, bad := range []string{"61", "-1"} {
		_, err = Load(write(t, "[plan]\nnotify_before = "+bad+"\n"))
		want := "config.toml: [plan] notify_before = " + bad + ", want 0 to 60 minutes"
		if err == nil || err.Error() != want {
			t.Fatalf("notify_before = %s: err = %v, want %s", bad, err, want)
		}
	}
	_, err = Load(write(t, "[plan]\nbogus = 1\n"))
	if err == nil || !strings.Contains(err.Error(), "unknown key plan.bogus") {
		t.Fatalf("unknown key: err = %v", err)
	}
	file := filepath.Join(t.TempDir(), "todo.txt")
	c, err = Load(write(t, "[plan]\nfile = '"+file+"'\nnotify = false\n"))
	if err != nil || c.Plan.File != file || c.Plan.Notify {
		t.Fatalf("file + notify: %+v, err %v", c.Plan, err)
	}
}

func TestLoadPlanAutoStart(t *testing.T) {
	c, err := Load(write(t, ""))
	if err != nil || c.Plan.AutoStart {
		t.Fatalf("empty file: %+v, err %v, want auto_start off", c.Plan, err)
	}
	c, err = Load(write(t, "[plan]\nauto_start = true\n"))
	if err != nil || !c.Plan.AutoStart || !c.Plan.Notify {
		t.Fatalf("auto_start = true: %+v, err %v", c.Plan, err)
	}
	if _, err = Load(write(t, "[plan]\nauto_start = \"yes\"\n")); err == nil {
		t.Fatal("a non-bool auto_start loaded")
	}
}

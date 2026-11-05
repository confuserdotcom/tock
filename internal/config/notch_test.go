package config

import (
	"strings"
	"testing"
)

func TestLoadNotch(t *testing.T) {
	c, err := Load(write(t, ""))
	if err != nil {
		t.Fatal(err)
	}
	if !c.Notch.Enabled || c.Notch.ShowInRecordings || !c.Notch.AllDisplays || c.Notch.OpenOnHover {
		t.Fatalf("defaults = %+v", c.Notch)
	}
	c, err = Load(write(t, "[notch]\nenabled = false\nshow_in_recordings = true\nall_displays = false\nopen_on_hover = true\n"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Notch.Enabled || !c.Notch.ShowInRecordings || c.Notch.AllDisplays || !c.Notch.OpenOnHover {
		t.Fatalf("set = %+v", c.Notch)
	}
	if _, err := Load(write(t, "[notch]\nshape = 1\n")); err == nil || !strings.Contains(err.Error(), "shape") {
		t.Fatalf("unknown key: err = %v", err)
	}
}

func TestNotchFeature(t *testing.T) {
	f := FeatureByID("notch")
	if f == nil || f.Table != "notch" || f.Key != "enabled" {
		t.Fatalf("feature = %+v", f)
	}
	if !f.On(Default()) {
		t.Fatal("notch off by default")
	}
}

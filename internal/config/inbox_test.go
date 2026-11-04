package config

import (
	"strings"
	"testing"
)

func TestLoadInbox(t *testing.T) {
	c, err := Load(write(t, ""))
	if err != nil || c.Inbox.MinMS != 2*60_000 || c.Inbox.AutoShare != 0.85 {
		t.Fatalf("default = %+v, %v", c.Inbox, err)
	}
	if c, err = Load(write(t, "[inbox]\nmin_minutes = 0\n")); err != nil || c.Inbox.MinMS != 0 {
		t.Fatalf("0 = %+v, %v", c.Inbox, err)
	}
	if c, err = Load(write(t, "[inbox]\nauto_accept = 0\n")); err != nil || c.Inbox.AutoShare != 0 {
		t.Fatalf("auto 0 = %+v, %v", c.Inbox, err)
	}
	for _, in := range []string{"min_minutes = -1", "min_minutes = 61", "auto_accept = 59", "auto_accept = 101"} {
		if _, err := Load(write(t, "[inbox]\n"+in+"\n")); err == nil || !strings.Contains(err.Error(), "want 0") {
			t.Errorf("%s: err = %v", in, err)
		}
	}
}

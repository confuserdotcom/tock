package config

import (
	"strings"
	"testing"
)

func TestLoadInbox(t *testing.T) {
	c, err := Load(write(t, ""))
	if err != nil || c.Inbox.MinMS != 2*60_000 {
		t.Fatalf("default = %+v, %v", c.Inbox, err)
	}
	if c, err = Load(write(t, "[inbox]\nmin_minutes = 0\n")); err != nil || c.Inbox.MinMS != 0 {
		t.Fatalf("0 = %+v, %v", c.Inbox, err)
	}
	for _, in := range []string{"min_minutes = -1", "min_minutes = 61"} {
		if _, err := Load(write(t, "[inbox]\n"+in+"\n")); err == nil || !strings.Contains(err.Error(), "want 0 to 60") {
			t.Errorf("%s: err = %v", in, err)
		}
	}
}

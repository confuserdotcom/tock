package main

import "testing"

func TestManagerStopping(t *testing.T) {
	for out, want := range map[string]bool{
		"stopping\n": true, "running\n": false, "degraded\n": false, "": false, "starting\n": false,
	} {
		if got := managerStopping(out); got != want {
			t.Errorf("managerStopping(%q) = %v, want %v", out, got, want)
		}
	}
}

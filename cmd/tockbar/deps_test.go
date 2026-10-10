package main

import (
	"os/exec"
	"strings"
	"testing"
)

// The menu bar app runs all day: it must never be able to reach the
// network.
func TestTockbarLinksNoNetworkClient(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", ".").Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	deps := strings.Fields(string(out))
	if len(deps) < 10 {
		t.Fatalf("go list returned too little: %q", out)
	}
	for _, dep := range deps {
		switch dep {
		case "net/http", "crypto/tls", "tock/internal/claude":
			t.Errorf("tockbar depends on %s", dep)
		}
	}
}

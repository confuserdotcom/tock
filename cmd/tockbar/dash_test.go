package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestHelperWeb stands in for tock web when run by fakeWeb.
func TestHelperWeb(t *testing.T) {
	mode := os.Getenv("TOCKBAR_FAKE_WEB")
	if mode == "" {
		return
	}
	switch mode {
	case "serve":
		fmt.Printf("tock web: http://127.0.0.1:4242/#%s\nOnly this machine can connect.\n", strings.Repeat("ab", 32))
		time.Sleep(time.Minute)
	case "exit":
		fmt.Printf("tock web: http://127.0.0.1:4243/#%s\n", strings.Repeat("cd", 32))
	case "garbage":
		fmt.Println("tock: no database")
	}
	os.Exit(0)
}

func fakeWeb(mode string, starts *int) func() *exec.Cmd {
	return func() *exec.Cmd {
		*starts++
		c := exec.Command(os.Args[0], "-test.run=^TestHelperWeb$")
		c.Env = append(os.Environ(), "TOCKBAR_FAKE_WEB="+mode)
		return c
	}
}

func TestDashStartsOnce(t *testing.T) {
	starts := 0
	d := &dash{cmd: fakeWeb("serve", &starts)}
	t.Cleanup(d.Close)
	link, err := d.URL("")
	if err != nil {
		t.Fatal(err)
	}
	tok := strings.Repeat("ab", 32)
	if link != "http://127.0.0.1:4242/#"+tok {
		t.Fatalf("link = %q", link)
	}
	inbox, err := d.URL("inbox")
	if err != nil || inbox != "http://127.0.0.1:4242/?v=inbox#"+tok {
		t.Fatalf("inbox = %q, %v", inbox, err)
	}
	if starts != 1 {
		t.Fatalf("started %d times, want 1", starts)
	}
	d.Close()
	if _, err := d.URL(""); err != nil || starts != 2 {
		t.Fatalf("after close: starts %d, %v", starts, err)
	}
}

func TestDashRestartsAfterExit(t *testing.T) {
	starts := 0
	d := &dash{cmd: fakeWeb("exit", &starts)}
	t.Cleanup(d.Close)
	if _, err := d.URL(""); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		d.mu.Lock()
		gone := d.proc == nil
		d.mu.Unlock()
		if gone || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := d.URL(""); err != nil || starts != 2 {
		t.Fatalf("an exited dashboard must restart: starts %d, %v", starts, err)
	}
}

func TestDashNoLink(t *testing.T) {
	starts := 0
	d := &dash{cmd: fakeWeb("garbage", &starts)}
	if _, err := d.URL(""); err == nil || !strings.Contains(err.Error(), "no link") {
		t.Fatalf("err = %v", err)
	}
	missing := &dash{cmd: func() *exec.Cmd { return exec.Command("/nonexistent/tock", "web") }}
	if _, err := missing.URL(""); err == nil {
		t.Fatal("missing binary: no error")
	}
}

//go:build darwin && cgo

package collect

import (
	"os"
	"testing"
	"time"

	"tock/internal/span"
)

// TestMain runs the tests on a goroutine while the main OS thread (locked by
// the package init) pumps its run loop, so NSAppleScript calls dispatched to
// the main queue can execute.
func TestMain(m *testing.M) {
	done := make(chan int, 1)
	go func() { done <- m.Run() }()
	for {
		select {
		case code := <-done:
			os.Exit(code)
		default:
			runLoopSlice(50 * time.Millisecond)
		}
	}
}

func TestScriptSelfTestURL(t *testing.T) {
	out, errnum, ok := runScript(scriptSelftestURL, false)
	if !ok {
		t.Fatalf("runScript(selftest_url) failed with errnum %d", errnum)
	}
	got, ok := span.URLKey(out)
	if !ok || got != "example.com:8080/a/b" {
		t.Fatalf("URLKey(%q) = %q, %v; want example.com:8080/a/b", out, got, ok)
	}
}

func TestScriptSelfTestError(t *testing.T) {
	_, errnum, ok := runScript(scriptSelftestError, false)
	if ok {
		t.Fatal("runScript(selftest_error) succeeded; want failure")
	}
	if errnum != aeErrTimeout {
		t.Fatalf("errnum = %d; want %d", errnum, aeErrTimeout)
	}
}

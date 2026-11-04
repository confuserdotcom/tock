package main

import (
	"context"
	"testing"
	"time"

	"tock/internal/web"
)

func TestWebUsage(t *testing.T) {
	te := newTestEnv(t, testRules)
	for _, args := range [][]string{{"web", "x"}, {"web", "--port", "-1"}, {"web", "--port", "70000"}, {"web", "--nope"}, {"web", "--idle", "-1m"}} {
		if code, _ := te.run(t, "", args...); code != 2 {
			t.Errorf("%v: exit %d", args, code)
		}
	}
}

func TestStopWhenIdle(t *testing.T) {
	var act web.Activity
	now := time.Now()
	act.Touch(now)
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	done := make(chan struct{})
	go func() { stopWhenIdle(ctx, stop, &act, 50*time.Millisecond, time.Now); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("did not stop after idle")
	}
	if ctx.Err() == nil {
		t.Fatal("stop not called")
	}
}

package main

import (
	"context"
	"os/exec"
	"time"
)

// Notifier shows one notification.
type Notifier interface {
	Notify(title, body string) error
}

// notifyTimeout bounds one osascript run.
const notifyTimeout = 5 * time.Second

// osascriptNotifier shows notifications with osascript's display
// notification. Title and body go in as argv, never into the script text,
// so nothing in them can run as AppleScript.
type osascriptNotifier struct{}

func notifyArgs(title, body string) []string {
	return []string{
		"-e", "on run argv",
		"-e", "display notification (item 2 of argv) with title (item 1 of argv)",
		"-e", "end run",
		title, body,
	}
}

func (osascriptNotifier) Notify(title, body string) error {
	ctx, cancel := context.WithTimeout(context.Background(), notifyTimeout)
	defer cancel()
	return exec.CommandContext(ctx, "/usr/bin/osascript", notifyArgs(title, body)...).Run()
}

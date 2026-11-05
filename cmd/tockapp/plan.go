// Command tockapp is the tock.app in the .dmg: drag it to Applications and
// open it. The first time it sets tock up from the files inside it (tock
// setup --from); after that it opens the dashboard, or updates tock when
// this copy is newer than the one installed.
package main

import (
	"path/filepath"
	"strings"
)

// version is set via -ldflags "-X main.version=...".
var version = "dev"

// plan is what opening the app does.
type plan struct {
	kind        string // "open" runs cmd; "stop" only says text; "setup" asks, then runs cmd
	title, text string
	ok          string   // the button that goes ahead
	cmd         []string // the command to run
}

// payloadDir is the folder inside the app with what tock setup installs.
func payloadDir(self string) string {
	return filepath.Join(filepath.Dir(filepath.Dir(self)), "Resources", "libexec")
}

// decide picks the plan for the app at self (its executable) of version
// have, with installed the version already set up ("" for none) and
// payloadOK whether the files to install are in the app.
func decide(self, have, installed string, payloadOK bool) plan {
	if strings.Contains(self, "/AppTranslocation/") || strings.HasPrefix(self, "/Volumes/") {
		// opened from the disk image, or still quarantined where it was
		// downloaded: macOS runs it from a hidden copy, and the next open
		// would set up from somewhere else
		return plan{kind: "stop", title: "Move tock to Applications",
			text: "Drag tock into the Applications folder, then open it from there."}
	}
	if !payloadOK {
		return plan{kind: "stop", title: "This copy of tock is incomplete",
			text: "Download it again from github.com/confuserdotcom/tock/releases."}
	}
	if installed != "" && installed == have {
		return plan{kind: "open", cmd: []string{"/usr/bin/open", "tock://dashboard"}}
	}
	from := payloadDir(self)
	run := []string{filepath.Join(from, "tock"), "setup", "--from", from, "--cli"}
	if installed == "" {
		return plan{kind: "setup", title: "Set up tock", ok: "Set Up", cmd: run,
			text: "tock puts its recorder, trackd, and the menu bar app in your home folder and starts them.\n\n" +
				"macOS asks for your password once, to trust the certificate tock signs them with. " +
				"The certificate is made on this Mac and stays in your keychain. Nothing you do leaves this Mac."}
	}
	return plan{kind: "setup", title: "Update tock", ok: "Update", cmd: run,
		text: "tock " + installed + " is set up. Replace it with " + have + "?\n\nYour data, rules and settings stay as they are."}
}

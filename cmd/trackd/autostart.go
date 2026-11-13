package main

import "strings"

// runKeyName is trackd's value under HKCU\Software\Microsoft\Windows\
// CurrentVersion\Run: what Windows runs when you sign in, no admin needed.
const runKeyName = "tock trackd"

// runKeyCommand is that value for the trackd.exe at exe: the tock.exe
// beside it, which starts trackd detached and with no console (tock
// start), so signing in shows a console for a moment at most.
func runKeyCommand(exe string) string {
	dir := exe[:strings.LastIndexAny(exe, `\/`)+1]
	return `"` + dir + `tock.exe" start`
}

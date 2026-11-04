package classify

import (
	"strings"

	"tock/internal/span"
)

// Interval is [From, To) in unix ms.
type Interval struct{ From, To int64 }

// DefaultCallApps are the apps that are the call itself when in front during
// a call: macOS bundle ids and Linux app ids. Chat apps that are mostly
// typed in (Slack) are left out.
var DefaultCallApps = []string{
	"com.apple.FaceTime", "us.zoom.xos", "com.microsoft.teams", "com.microsoft.teams2",
	"com.hnc.Discord", "ru.keepcoder.Telegram", "org.telegram.desktop", "net.whatsapp.WhatsApp",
	"desktop.WhatsApp", "com.skype.skype", "org.whispersystems.signal-desktop",
	"discord", "vesktop", "zoom", "signal",
}

// isCallView reports whether v, in front during a call, is the call itself:
// call time, a call app, or a meeting site (Meet, Zoom, Teams).
func (rs *Ruleset) isCallView(v *SpanView) bool {
	if v.BundleID == span.CallBundleID || rs.CallApps[v.BundleID] {
		return true
	}
	if v.Private || v.URL == nil {
		return false
	}
	host, _, _ := strings.Cut(*v.URL, "/")
	return isMeetingHost(strings.ToLower(host))
}

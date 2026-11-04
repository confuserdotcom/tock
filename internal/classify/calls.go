package classify

import (
	"strings"

	"tock/internal/span"
)

// Interval is [From, To) in unix ms.
type Interval struct{ From, To int64 }

// MarkCalls splits views at call boundaries. Pieces inside a call get
// InCall, and an idle piece inside a call becomes a span.CallBundleID view:
// a call you only listen to is not idle. Every piece keeps its span's ID.
// calls must be sorted by From and must not overlap.
func MarkCalls(views []SpanView, calls []Interval) []SpanView {
	if len(calls) == 0 {
		return views
	}
	out := make([]SpanView, 0, len(views)+2*len(calls))
	for _, v := range views {
		at := v.Start
		for _, c := range calls {
			if c.To <= at || c.From >= v.End {
				continue
			}
			if c.From > at {
				p := v
				p.Start, p.End = at, c.From
				out = append(out, p)
				at = c.From
			}
			p := v
			p.Start, p.End, p.InCall = at, min(c.To, v.End), true
			if p.BundleID == span.IdleBundleID {
				p.BundleID, p.App, p.Title, p.URL = span.CallBundleID, span.CallApp, nil, nil
			}
			out = append(out, p)
			at = p.End
		}
		if at < v.End {
			p := v
			p.Start = at
			out = append(out, p)
		}
	}
	return out
}

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

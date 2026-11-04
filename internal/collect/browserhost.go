package collect

import (
	"strings"

	"tock/internal/fronttab"
	"tock/internal/span"
)

// browserClasses are the Hyprland classes of the browsers the extension
// runs in, lowercased.
var browserClasses = map[string]bool{
	"chromium": true, "chromium-browser": true, "google-chrome": true,
	"brave-browser": true, "firefox": true, "org.mozilla.firefox": true,
}

// applyFrontTab adds the front tab's address to a browser sample whose
// window title starts with the tab's title, and marks it private when the
// extension says the front window is incognito or one it cannot see,
// provided it said so since this window was activated (sinceMS): an older
// verdict was about another window.
func applyFrontTab(smp *span.Sample, st fronttab.State, sinceMS int64) {
	if !browserClasses[strings.ToLower(smp.BundleID)] || smp.Title == nil {
		return
	}
	fresh := st.AtMS >= sinceMS-fronttab.SkewMS
	if (st.Private || !st.Focused) && fresh {
		applyPrivacy(smp)
		return
	}
	if !st.Focused || st.Private || st.Title == "" || st.URL == "" || !strings.HasPrefix(*smp.Title, st.Title) {
		return
	}
	u := st.URL
	smp.URL = &u
}

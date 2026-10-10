package collect

import (
	"fmt"
	"strings"

	"tock/internal/span"
)

// closeKind is the close-tab script of each scriptable browser (browser.h).
var closeKind = map[browserKind]browserKind{
	browserChrome: 21,
	browserArc:    22,
	browserBrave:  23,
	browserSafari: 24,
}

// closeIfHost closes the front tab of the browser with bundle when its URL
// host is still host. It reads the front URL first, so a tab the user has
// left in the meantime stays open. ok is false when nothing was closed.
func closeIfHost(bundle, host string, run scriptRunner) (bool, error) {
	kind := browserForBundle(bundle)
	ck, scriptable := closeKind[kind]
	if !scriptable || host == "" {
		return false, nil
	}
	out, errnum, ok := run(kind, false)
	if !ok {
		return false, fmt.Errorf("front tab: AppleScript error %d", errnum)
	}
	key, ok := span.URLKey(out)
	if !ok {
		return false, nil // private window, no window or not a web page
	}
	h, _, _ := strings.Cut(key, "/")
	h, _, _ = strings.Cut(h, ":")
	if h != host {
		return false, nil
	}
	if _, errnum, ok := run(ck, false); !ok {
		return false, fmt.Errorf("close tab: AppleScript error %d", errnum)
	}
	return true, nil
}

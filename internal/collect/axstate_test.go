package collect

import "testing"

func TestAXTrackerStartsUnknownThenTrusted(t *testing.T) {
	var tr axTracker
	changed, trusted, stale := tr.observe(true, 0)
	if !changed || !trusted || stale {
		t.Fatalf("first observe(true,0) = changed=%v trusted=%v stale=%v; want true,true,false", changed, trusted, stale)
	}
	changed, trusted, _ = tr.observe(true, 0)
	if changed || !trusted {
		t.Fatalf("second observe(true,0) = changed=%v trusted=%v; want false,true", changed, trusted)
	}
}

func TestAXTrackerRevokedTransition(t *testing.T) {
	var tr axTracker
	tr.observe(true, 0)
	changed, trusted, _ := tr.observe(false, 0)
	if !changed || trusted {
		t.Fatalf("revoke = changed=%v trusted=%v; want true,false", changed, trusted)
	}
	for i := 0; i < 3; i++ {
		changed, trusted, _ = tr.observe(false, 0)
		if changed || trusted {
			t.Fatalf("untrusted poll %d = changed=%v trusted=%v; want false,false", i, changed, trusted)
		}
	}
	changed, trusted, _ = tr.observe(true, 0)
	if !changed || !trusted {
		t.Fatalf("re-grant = changed=%v trusted=%v; want true,true", changed, trusted)
	}
}

func TestAXTrackerStaleAfterThreeAPIDisabled(t *testing.T) {
	var tr axTracker
	for i := 1; i <= 2; i++ {
		if _, _, stale := tr.observe(true, axErrAPIDisabled); stale {
			t.Fatalf("stale after %d disabled polls; want only on the third", i)
		}
	}
	if _, _, stale := tr.observe(true, axErrAPIDisabled); !stale {
		t.Fatal("third consecutive -25211 did not report stale")
	}

	// Any success resets the count.
	var tr2 axTracker
	tr2.observe(true, axErrAPIDisabled)
	tr2.observe(true, axErrAPIDisabled)
	tr2.observe(true, 0)
	tr2.observe(true, axErrAPIDisabled)
	if _, _, stale := tr2.observe(true, axErrAPIDisabled); stale {
		t.Fatal("stale after a success reset the run; want not stale")
	}
}

func TestAXTrackerOtherErrorsNotStale(t *testing.T) {
	var tr axTracker
	codes := []int{axErrCannotComplete, axErrNoValue, axErrAttributeUnsupported, axErrNotImplemented, axErrInvalidUIElement}
	for round := 0; round < 5; round++ {
		for _, c := range codes {
			if _, _, stale := tr.observe(true, c); stale {
				t.Fatalf("code %d made the tracker stale", c)
			}
		}
	}
}

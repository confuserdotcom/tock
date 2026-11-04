//go:build darwin && cgo

package collect

import (
	"slices"
	"testing"
)

// TestVisionRenderedText: Vision reads text drawn in memory (no capture, no
// permission needed).
func TestVisionRenderedText(t *testing.T) {
	words, reason := visionText("kubectl rollout deployment namespace")
	if reason != OCROK {
		t.Fatalf("reason = %s", reason)
	}
	for _, w := range []string{"kubectl", "rollout", "deployment"} {
		if !slices.Contains(words, w) {
			t.Errorf("missing %q in %q", w, words)
		}
	}
}

package collect

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"
	"unicode/utf8"
)

// Skip reasons of a screen read (OCR-02). "ok" is a read that happened.
const (
	OCROK           = "ok"
	OCROff          = "off"
	OCRNoPermission = "no_permission"
	OCRPrivate      = "private"
	OCRDeny         = "deny"
	OCRLocked       = "locked"
	OCRAway         = "afk"
	OCRSecure       = "secure"
	OCRDwell        = "dwell"
	OCRThrottle     = "throttle"
	OCRBusy         = "busy"
	OCRChanged      = "changed"   // the front window changed before the read
	OCRNoWindow     = "no_window" // the front app shows no window
	OCRUnavailable  = "unavailable"
	OCRError        = "error"
)

// Exit codes of "trackd ocr-worker" besides 0 (read done).
const (
	ocrExitNoPermission = 10
	ocrExitSecure       = 11
	ocrExitChanged      = 12
	ocrExitNoWindow     = 13
	ocrExitUnavailable  = 14
	ocrExitError        = 15
)

// OCRExit is the worker's exit code for a skip reason (0 for none).
func OCRExit(reason string) int {
	switch reason {
	case "", OCROK:
		return 0
	case OCRNoPermission:
		return ocrExitNoPermission
	case OCRSecure:
		return ocrExitSecure
	case OCRChanged:
		return ocrExitChanged
	case OCRNoWindow:
		return ocrExitNoWindow
	case OCRUnavailable:
		return ocrExitUnavailable
	}
	return ocrExitError
}

func ocrReason(code int) string {
	switch code {
	case 0:
		return OCROK
	case ocrExitNoPermission:
		return OCRNoPermission
	case ocrExitSecure:
		return OCRSecure
	case ocrExitChanged:
		return OCRChanged
	case ocrExitNoWindow:
		return OCRNoWindow
	case ocrExitUnavailable:
		return OCRUnavailable
	}
	return OCRError
}

// OCRTimeout is the hard limit of one read: the worker is killed after it.
// A cold accurate read of a large window takes up to about 2 s.
const OCRTimeout = 5 * time.Second

// ocrMaxOutput bounds what a worker may print.
const ocrMaxOutput = 64 << 10

// OCRResult is one read: the lines of text, or why there are none.
type OCRResult struct {
	Reason string // OCROK or a skip reason
	Lines  []string
	CostMS int64
}

// Capturer reads the text of the front window of bundle.
type Capturer interface {
	Capture(ctx context.Context, bundle string) OCRResult
}

// WorkerCapturer runs "Exe [Prefix...] ocr-worker --bundle B" once per read:
// screenshots and Vision live and die in that child, which is killed at
// OCRTimeout, so trackd keeps no pixels and none of Vision's memory. The
// text comes back through a pipe, never a file.
type WorkerCapturer struct {
	Exe    string
	Prefix []string // tests only
	Env    []string // tests only; nil = empty environment
	Now    func() time.Time
}

// Capture runs the worker for bundle.
func (w WorkerCapturer) Capture(ctx context.Context, bundle string) OCRResult {
	return w.run(ctx, "--bundle", bundle)
}

// Grant asks macOS for the Screen Recording permission through the worker,
// so the grant is trackd's. It reports whether it is granted now.
func (w WorkerCapturer) Grant(ctx context.Context) (bool, error) {
	r := w.run(ctx, "--grant")
	if r.Reason == OCRError {
		return false, errors.New("collect: ocr grant: worker failed")
	}
	return r.Reason == OCROK, nil
}

func (w WorkerCapturer) run(ctx context.Context, args ...string) OCRResult {
	now := w.Now
	if now == nil {
		now = time.Now
	}
	ctx, cancel := context.WithTimeout(ctx, OCRTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, w.Exe, append(append(append([]string{}, w.Prefix...), "ocr-worker"), args...)...)
	cmd.Env = w.Env
	if cmd.Env == nil {
		cmd.Env = []string{}
	}
	var out limitedBuffer
	cmd.Stdout = &out // stderr discarded: nothing read on screen goes to a log
	t0 := now()
	err := cmd.Run()
	r := OCRResult{CostMS: now().Sub(t0).Milliseconds()}
	var ee *exec.ExitError
	switch {
	case err == nil:
		r.Reason = OCROK
	case ctx.Err() != nil:
		r.Reason = OCRError
	case errors.As(err, &ee):
		r.Reason = ocrReason(ee.ExitCode())
	default:
		r.Reason = OCRError
	}
	if r.Reason != OCROK || out.over {
		if out.over {
			r.Reason = OCRError
		}
		return r
	}
	for _, l := range strings.Split(out.String(), "\n") {
		if l = strings.TrimSpace(l); l != "" && utf8.ValidString(l) {
			r.Lines = append(r.Lines, l)
		}
	}
	return r
}

// limitedBuffer keeps at most ocrMaxOutput bytes and remembers an overflow.
// The buffer is a field, not embedded, so io.Copy cannot bypass Write
// through bytes.Buffer.ReadFrom.
type limitedBuffer struct {
	buf  bytes.Buffer
	over bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.buf.Len()+len(p) > ocrMaxOutput {
		b.over = true
		return len(p), nil
	}
	return b.buf.Write(p)
}

func (b *limitedBuffer) String() string { return b.buf.String() }

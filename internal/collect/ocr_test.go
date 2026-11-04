package collect

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestHelperProcess is the fake ocr-worker: TOCK_FAKE_OCR says what to do.
func TestHelperProcess(t *testing.T) {
	mode := os.Getenv("TOCK_FAKE_OCR")
	if mode == "" {
		return
	}
	args := os.Args
	i := slices.Index(args, "ocr-worker")
	if i < 0 {
		os.Exit(99)
	}
	switch mode {
	case "text":
		if !slices.Equal(args[i+1:], []string{"--bundle", "com.x"}) {
			os.Exit(98)
		}
		fmt.Println("kubectl rollout\n\n  deployment  ")
		fmt.Fprintln(os.Stderr, "secret text that must not be read")
		os.Exit(0)
	case "secure":
		os.Exit(OCRExit(OCRSecure))
	case "noperm":
		os.Exit(OCRExit(OCRNoPermission))
	case "grant":
		if !slices.Equal(args[i+1:], []string{"--grant"}) {
			os.Exit(98)
		}
		os.Exit(0)
	case "hang":
		time.Sleep(time.Minute)
	case "flood":
		os.Stdout.WriteString(strings.Repeat("a", ocrMaxOutput+10))
		os.Exit(0)
	}
	os.Exit(1)
}

func fakeWorker(mode string) WorkerCapturer {
	return WorkerCapturer{Exe: os.Args[0], Prefix: []string{"-test.run=TestHelperProcess", "--"},
		Env: []string{"TOCK_FAKE_OCR=" + mode}}
}

func TestWorkerCapturer(t *testing.T) {
	ctx := context.Background()
	r := fakeWorker("text").Capture(ctx, "com.x")
	if r.Reason != OCROK || !slices.Equal(r.Lines, []string{"kubectl rollout", "deployment"}) {
		t.Fatalf("text = %+v", r)
	}
	for mode, want := range map[string]string{"secure": OCRSecure, "noperm": OCRNoPermission, "flood": OCRError, "crash": OCRError} {
		if r := fakeWorker(mode).Capture(ctx, "com.x"); r.Reason != want || r.Lines != nil {
			t.Errorf("%s = %+v, want %s", mode, r, want)
		}
	}
	if ok, err := fakeWorker("grant").Grant(ctx); err != nil || !ok {
		t.Errorf("grant = %v, %v", ok, err)
	}
	if ok, err := fakeWorker("noperm").Grant(ctx); err != nil || ok {
		t.Errorf("grant refused = %v, %v", ok, err)
	}
}

func TestWorkerCapturerTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("waits OCRTimeout")
	}
	t0 := time.Now()
	r := fakeWorker("hang").Capture(context.Background(), "com.x")
	if r.Reason != OCRError || time.Since(t0) > OCRTimeout+2*time.Second {
		t.Fatalf("hang = %+v after %v", r, time.Since(t0))
	}
}

func TestOCRExitRoundTrip(t *testing.T) {
	for _, r := range []string{OCROK, OCRNoPermission, OCRSecure, OCRChanged, OCRNoWindow, OCRUnavailable, OCRError} {
		if got := ocrReason(OCRExit(r)); got != r {
			t.Errorf("%s -> %d -> %s", r, OCRExit(r), got)
		}
	}
}

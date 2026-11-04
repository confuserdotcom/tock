package fronttab

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func frame(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	binary.Write(&out, binary.LittleEndian, uint32(len(b)))
	out.Write(b)
	return out.Bytes()
}

func readState(t *testing.T, path string) State {
	t.Helper()
	st, ok := Read(path)
	if !ok {
		t.Fatal("no state")
	}
	return st
}

func TestBrowserHostWritesTheFrontTab(t *testing.T) {
	path := filepath.Join(t.TempDir(), File)
	in := frame(t, map[string]any{"focused": true, "url": "https://user:pw@www.Chess.com/play/online?token=x#y", "title": "Play"})
	if err := RunHost(bytes.NewReader(in), path, func() int64 { return 42 }); err != nil {
		t.Fatal(err)
	}
	st := readState(t, path)
	if st.AtMS != 42 || st.URL != "www.chess.com/play/online" || st.Title != "Play" || !st.Focused || st.Private {
		t.Fatalf("state = %+v", st)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", fi.Mode())
	}
	// The last message wins; an incognito tab leaves no address or title on disk.
	in = append(frame(t, map[string]any{"focused": true, "url": "https://a.test/", "title": "A"}),
		frame(t, map[string]any{"focused": true, "url": "https://secret.test/x", "title": "Secret", "incognito": true})...)
	if err := RunHost(bytes.NewReader(in), path, func() int64 { return 43 }); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if st := readState(t, path); !st.Private || st.URL != "" || st.Title != "" || strings.Contains(string(b), "ecret") {
		t.Fatalf("incognito state = %s", b)
	}
	// Non-web pages keep the title but no address.
	in = frame(t, map[string]any{"focused": true, "url": "chrome://settings", "title": "Settings"})
	RunHost(bytes.NewReader(in), path, func() int64 { return 44 })
	if st := readState(t, path); st.URL != "" || st.Title != "Settings" {
		t.Fatalf("chrome:// state = %+v", st)
	}
}

func TestBrowserHostRejectsBadFrames(t *testing.T) {
	path := filepath.Join(t.TempDir(), File)
	var big bytes.Buffer
	binary.Write(&big, binary.LittleEndian, uint32(MaxMsg+1))
	if err := RunHost(&big, path, func() int64 { return 1 }); err == nil {
		t.Fatal("oversized frame accepted")
	}
	if err := RunHost(bytes.NewReader([]byte{3, 0, 0, 0, '{', 'x', '}'}), path, func() int64 { return 1 }); err == nil {
		t.Fatal("bad JSON accepted")
	}
	if err := RunHost(bytes.NewReader([]byte{9, 0, 0, 0, '{'}), path, func() int64 { return 1 }); err == nil {
		t.Fatal("short frame accepted")
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("a bad frame wrote state")
	}
	if err := RunHost(bytes.NewReader(nil), path, func() int64 { return 1 }); err != nil {
		t.Fatalf("EOF is a clean exit: %v", err)
	}
}

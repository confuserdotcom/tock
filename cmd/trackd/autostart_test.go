package main

import "testing"

func TestRunKeyCommand(t *testing.T) {
	got := runKeyCommand(`C:\Users\a b\tock\trackd.exe`)
	if want := `"C:\Users\a b\tock\tock.exe" start`; got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

//go:build !darwin || !cgo

package main

// save_darwin.go only gives save_darwin.c a Go face for its tests; the
// dashboard window, and so downloads, exist only on macOS.

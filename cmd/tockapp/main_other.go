//go:build !darwin || !cgo

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "tockapp is the macOS app in the .dmg; on Linux run tock setup")
	os.Exit(1)
}

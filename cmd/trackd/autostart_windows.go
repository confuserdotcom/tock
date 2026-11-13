//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`

// installAutostart makes Windows start trackd when you sign in, through
// the tock.exe next to this trackd.exe, and starts it now.
func installAutostart() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	tock := filepath.Join(filepath.Dir(exe), "tock.exe")
	if _, err := os.Stat(tock); err != nil {
		return fmt.Errorf("tock.exe not found next to trackd.exe (%s): keep the two in one folder", tock)
	}
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open the Run key: %w", err)
	}
	defer k.Close()
	if err := k.SetStringValue(runKeyName, runKeyCommand(exe)); err != nil {
		return fmt.Errorf("write the Run key: %w", err)
	}
	fmt.Println("trackd starts when you sign in (trackd uninstall undoes it)")
	out, err := exec.Command(tock, "start").CombinedOutput()
	os.Stdout.Write(out)
	return err
}

// uninstallAutostart stops trackd starting at sign-in. It leaves a running
// trackd alone (tock stop) and keeps tock.db.
func uninstallAutostart() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open the Run key: %w", err)
	}
	defer k.Close()
	if err := k.DeleteValue(runKeyName); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return fmt.Errorf("remove the Run key value: %w", err)
	}
	return nil
}

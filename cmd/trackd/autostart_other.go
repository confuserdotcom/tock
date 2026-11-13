//go:build !windows

package main

import "errors"

func installAutostart() error   { return errors.New("only on Windows") }
func uninstallAutostart() error { return errors.New("only on Windows") }

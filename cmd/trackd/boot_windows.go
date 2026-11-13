//go:build windows

package main

import (
	"time"

	"golang.org/x/sys/windows"
)

var procGetTickCount64 = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetTickCount64")

// bootMS is when the computer last started, in unix ms. GetTickCount64
// counts from boot and includes sleep, like Linux's uptime.
func bootMS() int64 {
	ms, _, _ := procGetTickCount64.Call()
	return time.Now().UnixMilli() - int64(ms)
}

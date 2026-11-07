package main

import (
	"syscall"
	"time"
)

// bootMS is when the computer last started, in unix ms; 0 if unknown. The
// uptime counts suspended time, so it holds across sleep; it has whole
// seconds only.
func bootMS() int64 {
	var si syscall.Sysinfo_t
	if err := syscall.Sysinfo(&si); err != nil {
		return 0
	}
	return time.Now().UnixMilli() - int64(si.Uptime)*1000
}

package main

import "golang.org/x/sys/unix"

// bootMS is when the computer last started, in unix ms; 0 if unknown.
func bootMS() int64 {
	tv, err := unix.SysctlTimeval("kern.boottime")
	if err != nil {
		return 0
	}
	return tv.Sec*1000 + int64(tv.Usec)/1000
}

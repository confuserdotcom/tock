//go:build !unix

package main

import "errors"

// privateFiles does nothing: files under the user profile are private to
// the user by their inherited ACL.
func privateFiles() {}

// execInto is unavailable: there is no exec without a new pid here.
func execInto(string) error { return errors.New("exec is not supported on this platform") }

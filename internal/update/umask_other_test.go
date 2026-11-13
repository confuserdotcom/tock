//go:build !unix

package update

func setUmask(int) int { return 0 }

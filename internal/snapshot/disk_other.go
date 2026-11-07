//go:build !unix

package snapshot

// FreeDisk is unknown here; report plenty so snapshots still run.
func FreeDisk(string) (uint64, error) { return 1 << 62, nil }

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// listingTTLMS is how long the numbers printed by inbox/today --keys stay
// valid for tag and ignore.
const listingTTLMS = 60 * 60 * 1000

var errListing = errors.New("listing expired: run tock inbox again")

type listing struct {
	Created int64          `json:"created"`
	Entries []listingEntry `json:"entries"`
}

type listingEntry struct {
	N   int    `json:"n"`
	Key string `json:"key"`
}

// saveListing writes the numbered keys (0600; they may contain titles).
func saveListing(env Env, keys []string) error {
	l := listing{Created: env.Now().UnixMilli()}
	for i, k := range keys {
		l.Entries = append(l.Entries, listingEntry{N: i + 1, Key: k})
	}
	data, err := json.Marshal(l)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(env.listingPath()), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(env.listingPath(), data, 0o600); err != nil {
		return err
	}
	return os.Chmod(env.listingPath(), 0o600)
}

// resolveListed maps a number from the last listing to its key.
func resolveListed(env Env, arg string) (string, error) {
	n, err := strconv.Atoi(arg)
	if err != nil || n < 1 {
		return "", usagef("%q is not an item number from tock inbox", arg)
	}
	data, err := os.ReadFile(env.listingPath())
	if errors.Is(err, os.ErrNotExist) {
		return "", errListing
	}
	if err != nil {
		return "", err
	}
	var l listing
	if err := json.Unmarshal(data, &l); err != nil {
		return "", errListing
	}
	if env.Now().UnixMilli()-l.Created > listingTTLMS {
		return "", errListing
	}
	for _, e := range l.Entries {
		if e.N == n {
			return e.Key, nil
		}
	}
	return "", fmt.Errorf("no item %d in the last listing", n)
}

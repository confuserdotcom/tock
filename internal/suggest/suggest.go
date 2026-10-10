// Package suggest builds the privacy-filtered payload tock sends to Claude
// for inbox suggestions and reads the answer back. It is pure: no network,
// no database, no clock. The network client lives in internal/claude, which
// only cmd/tock imports.
package suggest

import (
	"context"
	"encoding/json"
)

const (
	// BatchMax is the most keys one request carries.
	BatchMax = 40
	// EveryMS is the least time between two batches.
	EveryMS int64 = 60 * 60_000
	// InboxDays is the inbox window suggestions are drawn from.
	InboxDays = 7
)

// Client sends one payload and returns the input of the model's suggest
// tool call.
type Client interface {
	Suggest(ctx context.Context, model string, payload []byte) (json.RawMessage, error)
}

// Batch is one request: the exact bytes to send and the inbox key behind
// each batch-local id ("k1", "k2", ...).
type Batch struct {
	Keys    map[string]string
	Payload []byte
}

// Status says what SuggestPrepare found.
type Status int

const (
	Off     Status = iota // [llm] enabled = false
	Nothing               // no eligible key
	Wait                  // the hour is taken; NextMS says until when
	Ready                 // the hour is claimed; Batch is ready to send
)

// Prepared is the outcome of preparing a batch.
type Prepared struct {
	Status Status
	Batch  Batch
	Model  string
	NextMS int64
}

// Suggestion is Claude's answer for one key. Empty Category and Project
// mean no suggestion.
type Suggestion struct {
	Key, Category, Project string
}

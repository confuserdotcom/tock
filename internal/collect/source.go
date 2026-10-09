// Package collect produces span.Input values from the operating system.
package collect

import (
	"errors"
	"time"

	"tock/internal/span"
)

// DefaultPollInterval is the sample poll period (D-05).
const DefaultPollInterval = 2 * time.Second

// ChanBuffer is the capacity of the Inputs channel.
const ChanBuffer = 256

// ErrUnsupported is returned when no collector exists for this build.
var ErrUnsupported = errors.New("collect: unsupported platform or cgo disabled")

// Source produces inputs. Run blocks; the darwin implementation must be called
// from the main goroutine, which is locked to the main OS thread. Run closes
// the Inputs channel when it returns. Stop is safe from any goroutine and
// idempotent.
type Source interface {
	Inputs() <-chan span.Input
	Run() error
	Stop()
	Dropped() uint64
}

// Options configures a darwin Source.
type Options struct {
	PollInterval  time.Duration
	PromptAX      bool
	AskAutomation bool
}

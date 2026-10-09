package collect

import "tock/internal/span"

// Fake is a Source that replays a fixed list of inputs (D-08). Every
// downstream test drives the pipeline with it.
type Fake struct {
	inputs []span.Input
	ch     chan span.Input
}

// NewFake returns a Fake whose channel is sized to hold every input.
func NewFake(inputs []span.Input) *Fake {
	return &Fake{inputs: inputs, ch: make(chan span.Input, len(inputs)+1)}
}

func (f *Fake) Inputs() <-chan span.Input { return f.ch }

// Run sends every input in order, closes the channel and returns.
func (f *Fake) Run() error {
	for _, in := range f.inputs {
		f.ch <- in
	}
	close(f.ch)
	return nil
}

// Stop is a no-op, safe to call repeatedly.
func (f *Fake) Stop() {}

// Dropped always returns 0.
func (f *Fake) Dropped() uint64 { return 0 }

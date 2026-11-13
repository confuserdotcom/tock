//go:build !windows

package stopsignal

func listen(string) (<-chan struct{}, func(), error) { return nil, func() {}, ErrUnsupported }

func send(string) error { return ErrUnsupported }

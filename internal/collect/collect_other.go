//go:build !(darwin && cgo)

package collect

// NewDarwin is unavailable without darwin and cgo; callers get ErrUnsupported
// so cmd/trackd still compiles with CGO_ENABLED=0.
func NewDarwin(opts Options) (Source, error) {
	return nil, ErrUnsupported
}

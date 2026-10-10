package classify

func sp(s string) *string { return &s }

// optField turns the golden-file marker <nil> into a nil pointer.
func optField(s string) *string {
	if s == "<nil>" {
		return nil
	}
	return &s
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

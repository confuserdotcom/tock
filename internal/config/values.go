package config

import (
	"fmt"
	"strconv"
	"strings"
)

// SetString is SetBool for a string: the value is written as a TOML basic
// string.
func SetString(src []byte, table, key, v string) ([]byte, error) {
	b := setValue(src, table, key, tomlString(v))
	return check(src, b, table, key, func(got any) bool { return got == v })
}

// SetInt is SetBool for an integer.
func SetInt(src []byte, table, key string, v int) ([]byte, error) {
	b := setValue(src, table, key, strconv.Itoa(v))
	return check(src, b, table, key, func(got any) bool { return got == int64(v) })
}

// SetInts is SetList for a list of integers.
func SetInts(src []byte, table, key string, vals []int) ([]byte, error) {
	s := make([]string, len(vals))
	for i, v := range vals {
		s[i] = strconv.Itoa(v)
	}
	b := setValue(src, table, key, "["+strings.Join(s, ", ")+"]")
	return check(src, b, table, key, func(got any) bool {
		l, ok := got.([]any)
		if !ok || len(l) != len(vals) {
			return false
		}
		for i := range l {
			if l[i] != int64(vals[i]) {
				return false
			}
		}
		return true
	})
}

// tomlString quotes s as a TOML basic string.
func tomlString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		case '\r':
			b.WriteString(`\r`)
		default:
			if r < 0x20 || r == 0x7f {
				fmt.Fprintf(&b, `\u%04X`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

// Change is one key of config.toml to write: Value is a bool, a string, an
// int, a []string or an []int; nil removes the key.
type Change struct {
	Table, Key string
	Value      any
}

// SaveValues writes changes into the config.toml at path the way
// SaveFeatures does: everything else in the file stays, and nothing is
// written unless the result loads. The error then says what is wrong.
func SaveValues(path string, changes []Change) error {
	return saveEdit(path, "save settings", func(b []byte, _ Config) ([]byte, error) {
		return applyChanges(b, changes)
	})
}

// SaveInvoice is SaveValues that also replaces the [[invoice.client]]
// tables with clients.
func SaveInvoice(path string, changes []Change, clients []InvoiceClient) error {
	return saveEdit(path, "save invoice details", func(b []byte, _ Config) ([]byte, error) {
		b, err := applyChanges(b, changes)
		if err != nil {
			return nil, err
		}
		return SetClients(b, clients)
	})
}

func applyChanges(b []byte, changes []Change) ([]byte, error) {
	var err error
	for _, c := range changes {
		switch v := c.Value.(type) {
		case nil:
			b, err = UnsetKey(b, c.Table, c.Key)
		case bool:
			b, err = SetBool(b, c.Table, c.Key, v)
		case string:
			b, err = SetString(b, c.Table, c.Key, v)
		case int:
			b, err = SetInt(b, c.Table, c.Key, v)
		case []string:
			b, err = SetList(b, c.Table, c.Key, v)
		case []int:
			b, err = SetInts(b, c.Table, c.Key, v)
		default:
			return nil, fmt.Errorf("config: [%s] %s: can't write a %T", c.Table, c.Key, v)
		}
		if err != nil {
			return nil, err
		}
	}
	return b, nil
}

// SetClients returns src with every [[invoice.client]] table replaced by
// clients, written as a block at the end of the file. Comments inside the
// old client tables go with them; everything else stays.
func SetClients(src []byte, clients []InvoiceClient) ([]byte, error) {
	text := string(src)
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	lines := strings.SplitAfter(text, "\n")
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	var out []string
	in := false
	for _, l := range lines {
		if name, ok := header(l); ok {
			in = strings.HasPrefix(strings.TrimSpace(l), "[[") && name == "invoice.client"
			if in {
				// Drop the blank line that separated the table from the one before.
				if n := len(out); n > 0 && strings.TrimSpace(out[n-1]) == "" {
					out = out[:n-1]
				}
			}
		}
		if !in {
			out = append(out, l)
		}
	}
	for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}
	for _, c := range clients {
		if len(out) > 0 {
			out = append(out, "\n")
		}
		out = append(out, "[[invoice.client]]\n", "name = "+tomlString(c.Name)+"\n")
		if len(c.Address) > 0 {
			q := make([]string, len(c.Address))
			for i, a := range c.Address {
				q[i] = tomlString(a)
			}
			out = append(out, "address = ["+strings.Join(q, ", ")+"]\n")
		}
		if c.TaxID != "" {
			out = append(out, "tax_id = "+tomlString(c.TaxID)+"\n")
		}
	}
	b := []byte(strings.Join(out, ""))
	if _, err := Parse(File, b); err != nil {
		return nil, fmt.Errorf("config: invoice clients: %w", err)
	}
	return b, nil
}

// FormatTaxRate writes basis points as ParseTaxRate reads them: 2000 is
// "20", 550 is "5.5".
func FormatTaxRate(bp int64) string {
	s := fmt.Sprintf("%d.%02d", bp/100, bp%100)
	s = strings.TrimRight(s, "0")
	return strings.TrimSuffix(s, ".")
}

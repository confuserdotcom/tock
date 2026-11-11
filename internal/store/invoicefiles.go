package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// The invoices directory holds, per invoice, <number>.json (the canonical
// JSON), <number>.pdf (the stored PDF bytes) and, once voided,
// <number>.void.json. The database is the record; the files are the copy
// a person sends and the way back after restoring an older snapshot
// (RecoverInvoices). All 0600, in a 0700 directory.

// InvoiceDir is the invoices directory beside the database at dbPath.
func InvoiceDir(dbPath string) string { return filepath.Join(filepath.Dir(dbPath), "invoices") }

// voidDoc is <number>.void.json.
type voidDoc struct {
	Number string `json:"number"`
	VoidMS int64  `json:"void_ms"`
	Reason string `json:"reason"`
}

func invoiceFiles(inv Invoice) map[string][]byte {
	files := map[string][]byte{inv.Number + ".json": inv.CanonicalJSON, inv.Number + ".pdf": inv.PDF}
	if inv.Status == InvoiceVoid {
		b, _ := encodeCanonical(voidDoc{inv.Number, inv.VoidMS, inv.VoidReason})
		files[inv.Number+".void.json"] = b
	}
	return files
}

// WriteInvoiceFiles writes inv's files to dir, replacing them atomically.
// inv must carry its document (IssueInvoice or InvoiceByNumber).
func WriteInvoiceFiles(dir string, inv Invoice) error {
	if inv.CanonicalJSON == nil || inv.PDF == nil {
		return fmt.Errorf("store: invoice %s files: no document loaded", inv.Number)
	}
	for name, b := range invoiceFiles(inv) {
		if err := writeFileAtomic(dir, name, b); err != nil {
			return err
		}
	}
	return nil
}

func writeFileAtomic(dir, name string, b []byte) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("store: invoice files: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("store: invoice files: %w", err)
	}
	f, err := os.CreateTemp(dir, ".tmp-"+name+"-")
	if err != nil {
		return fmt.Errorf("store: invoice files: %w", err)
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	_, err = f.Write(b)
	if err == nil {
		err = f.Chmod(0o600)
	}
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp, filepath.Join(dir, name))
	}
	if err != nil {
		return fmt.Errorf("store: invoice file %s: %w", name, err)
	}
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		d.Close()
	}
	return nil
}

// RepairInvoiceFiles writes the files of every invoice that are missing
// from dir, from the database, and returns their names sorted. A file that
// exists is never overwritten, even when it differs: VerifyInvoices reports
// that.
func (s *Store) RepairInvoiceFiles(dir string) ([]string, error) {
	list, err := s.Invoices(0)
	if err != nil {
		return nil, err
	}
	var wrote []string
	for _, sum := range list {
		names := []string{sum.Number + ".json", sum.Number + ".pdf"}
		if sum.Status == InvoiceVoid {
			names = append(names, sum.Number+".void.json")
		}
		var missing []string
		for _, n := range names {
			if _, err := os.Stat(filepath.Join(dir, n)); errors.Is(err, os.ErrNotExist) {
				missing = append(missing, n)
			}
		}
		if len(missing) == 0 {
			continue
		}
		inv, err := s.InvoiceByNumber(sum.Number)
		if err != nil {
			return wrote, err
		}
		files := invoiceFiles(inv)
		for _, n := range missing {
			if err := writeFileAtomic(dir, n, files[n]); err != nil {
				return wrote, err
			}
			wrote = append(wrote, n)
		}
	}
	sort.Strings(wrote)
	return wrote, nil
}

// InvoiceCheck is what VerifyInvoices found for one invoice; no Problems
// means the stored hashes match the stored document and the files.
type InvoiceCheck struct {
	Number   string
	Problems []string
}

// OK reports whether nothing differs.
func (c InvoiceCheck) OK() bool { return len(c.Problems) == 0 }

// VerifyInvoices recomputes the hashes of the stored JSON and PDF and
// compares them with the stored hashes and with the files in dir, for one
// invoice or (number "") all of them.
func (s *Store) VerifyInvoices(dir, number string) ([]InvoiceCheck, error) {
	var numbers []string
	if number != "" {
		numbers = []string{number}
	} else {
		list, err := s.Invoices(0)
		if err != nil {
			return nil, err
		}
		for _, i := range list {
			numbers = append(numbers, i.Number)
		}
	}
	var out []InvoiceCheck
	for _, n := range numbers {
		inv, err := s.InvoiceByNumber(n)
		if err != nil {
			return out, err
		}
		c := InvoiceCheck{Number: inv.Number}
		if hash(inv.CanonicalJSON) != inv.JSONSHA256 {
			c.Problems = append(c.Problems, "stored json does not match its hash")
		}
		if hash(inv.PDF) != inv.PDFSHA256 {
			c.Problems = append(c.Problems, "stored pdf does not match its hash")
		}
		for name, want := range invoiceFiles(inv) {
			kind := strings.TrimPrefix(name, inv.Number+".")
			b, err := os.ReadFile(filepath.Join(dir, name))
			switch {
			case errors.Is(err, os.ErrNotExist):
				c.Problems = append(c.Problems, kind+" file missing")
			case err != nil:
				c.Problems = append(c.Problems, kind+" file: "+err.Error())
			case !bytes.Equal(b, want):
				c.Problems = append(c.Problems, kind+" file differs from the record")
			}
		}
		sort.Strings(c.Problems)
		out = append(out, c)
	}
	return out, nil
}

// RecoverInvoices imports the invoices in dir whose number the database
// lacks, with their voids, and returns the numbers imported. Each JSON must
// be canonical and its PDF must match the hash it records; otherwise
// nothing is imported. An existing number is never overwritten.
func (s *Store) RecoverInvoices(dir string) ([]string, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	type found struct {
		inv  Invoice
		void *voidDoc
	}
	var todo []found
	for _, p := range paths {
		if strings.HasSuffix(p, ".void.json") {
			continue
		}
		js, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("store: recover: %w", err)
		}
		var d invoiceDoc
		if err := json.Unmarshal(js, &d); err != nil {
			return nil, fmt.Errorf("store: recover %s: %w", filepath.Base(p), err)
		}
		if again, err := encodeCanonical(d); err != nil || !bytes.Equal(again, js) || d.Format != invoiceFormat {
			return nil, fmt.Errorf("store: recover %s: not a canonical invoice file", filepath.Base(p))
		}
		if d.Number+".json" != filepath.Base(p) {
			return nil, fmt.Errorf("store: recover %s: holds invoice %s", filepath.Base(p), d.Number)
		}
		if _, err := invoiceSummary(s.db, d.Number); err == nil {
			continue
		} else if !errors.Is(err, ErrNoInvoice) {
			return nil, err
		}
		inv := invoiceOfDoc(d)
		inv.Supersedes = d.Supersedes
		if inv.PDF, err = os.ReadFile(filepath.Join(dir, d.Number+".pdf")); err != nil {
			return nil, fmt.Errorf("store: recover %s: %w", d.Number, err)
		}
		if hash(inv.PDF) != d.PDFSHA256 {
			return nil, fmt.Errorf("store: recover %s: the pdf does not match the hash in its json", d.Number)
		}
		inv.CanonicalJSON, inv.JSONSHA256 = js, hash(js)
		f := found{inv: inv}
		if vb, err := os.ReadFile(filepath.Join(dir, d.Number+".void.json")); err == nil {
			var v voidDoc
			if err := json.Unmarshal(vb, &v); err != nil || v.Number != d.Number || strings.TrimSpace(v.Reason) == "" {
				return nil, fmt.Errorf("store: recover %s: bad void file", d.Number)
			}
			f.void = &v
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("store: recover %s: %w", d.Number, err)
		}
		todo = append(todo, f)
	}
	// Issue order, so a superseded invoice is in before its replacement.
	sort.SliceStable(todo, func(a, b int) bool {
		if todo[a].inv.IssuedMS != todo[b].inv.IssuedMS {
			return todo[a].inv.IssuedMS < todo[b].inv.IssuedMS
		}
		return todo[a].inv.Number < todo[b].inv.Number
	})
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("store: recover: %w", err)
	}
	defer tx.Rollback()
	var done []string
	for _, f := range todo {
		inv := f.inv
		if inv.Supersedes != "" {
			old, err := invoiceSummary(tx, inv.Supersedes)
			if err != nil {
				return nil, fmt.Errorf("store: recover %s: superseded %w", inv.Number, err)
			}
			inv.SupersedesID = old.ID
		}
		if _, err := insertInvoice(tx, inv); err != nil {
			return nil, err
		}
		if f.void != nil {
			if err := voidTx(tx, inv.Number, f.void.Reason, f.void.VoidMS); err != nil {
				return nil, err
			}
		}
		done = append(done, inv.Number)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("store: recover: %w", err)
	}
	sort.Strings(done)
	return done, nil
}

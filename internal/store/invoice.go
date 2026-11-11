package store

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"tock/internal/report"
)

// Issued invoices: rows in invoices, invoice_lines and
// invoice_items that the 0026 triggers keep as issued. The only change is
// a one-time void.

// ErrNoInvoice is returned for an invoice number the database lacks.
var ErrNoInvoice = errors.New("store: no such invoice")

// Invoice statuses.
const (
	InvoiceIssued = "issued"
	InvoiceVoid   = "void"
)

// invoiceFormat is the canonical JSON's format number.
const invoiceFormat = 1

// Invoice is one issued invoice. Lines, Items, CanonicalJSON and PDF are
// filled by InvoiceByNumber and IssueInvoice, not by Invoices.
type Invoice struct {
	ID     int64
	Number string
	Year   int
	Seq    int // 0 = a number chosen with --number
	Status string
	// IssuedMS is when the invoice was issued; the year is its local year.
	IssuedMS     int64
	SupersedesID int64  // 0 = none
	Supersedes   string // the superseded invoice's number
	VoidMS       int64
	VoidReason   string

	IssuerName    string
	IssuerAddress []string
	IssuerTaxID   string
	ClientName    string
	ClientAddress []string
	ClientTaxID   string

	Project                  string
	PeriodFromMS, PeriodToMS int64
	TZ                       string // IANA name the day lines use

	Currency   string
	MinorUnits int
	RateMinor  int64
	RoundStepS int64
	RoundMode  string

	TaxLabel string
	TaxBP    int64
	TaxNote  string

	SecondsBilled, SubtotalMinor, TaxMinor, TotalMinor int64

	Note      string
	Generator string // the tock version that issued it

	Lines []InvoiceLine
	Items []InvoiceItem

	CanonicalJSON []byte
	JSONSHA256    string
	PDF           []byte
	PDFSHA256     string
}

// InvoiceLine is one printed row: a local day.
type InvoiceLine struct {
	Day           string `json:"day"`
	SecondsRaw    int64  `json:"seconds_raw"`
	SecondsBilled int64  `json:"seconds_billed"`
	AmountMinor   int64  `json:"amount_minor"`
}

// InvoiceItem is billed time behind the lines. It never holds a title or
// URL.
type InvoiceItem struct {
	Origin         string `json:"origin"` // span, edit, away, meeting or remote
	OriginID       int64  `json:"origin_id"`
	StartMS        int64  `json:"start_ms"`
	EndMS          int64  `json:"end_ms"`
	Seconds        int64  `json:"seconds"`
	Category       string `json:"category"`
	Project        string `json:"project"`
	CategorySource string `json:"category_source"`
	ProjectSource  string `json:"project_source"`
}

// IssueSpec is an invoice to issue. Draft holds everything but the
// identity and the document: Number "" takes the next of the issue year's
// sequence. Loc gives that year. Render makes the PDF for the numbered
// invoice, inside the transaction. Supersedes names an invoice this one
// replaces: it must be void already, or VoidOldReason voids it in the same
// transaction.
type IssueSpec struct {
	Draft         Invoice
	Loc           *time.Location
	Supersedes    string
	VoidOldReason string
	Render        func(Invoice) ([]byte, error)
}

// invoiceHook lets tests fail an issue at a stage ("before-commit").
var invoiceHook func(stage string) error

// IssueInvoice stores sp.Draft as an issued invoice in one immediate
// transaction: number, PDF, canonical JSON and all rows, or nothing.
func (s *Store) IssueInvoice(sp IssueSpec) (Invoice, error) {
	inv := normalizeInvoice(sp.Draft)
	inv.Number = strings.TrimSpace(inv.Number)
	switch {
	case sp.Render == nil || sp.Loc == nil:
		return Invoice{}, errors.New("store: issue invoice: no renderer or location")
	case inv.Project == "" || inv.TZ == "" || inv.IssuedMS <= 0:
		return Invoice{}, errors.New("store: issue invoice: project, time zone and issue time are required")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return Invoice{}, fmt.Errorf("store: issue invoice: %w", err)
	}
	defer tx.Rollback()

	if n := strings.TrimSpace(sp.Supersedes); n != "" {
		old, err := invoiceSummary(tx, n)
		if err != nil {
			return Invoice{}, fmt.Errorf("store: supersede %s: %w", n, err)
		}
		if old.Status == InvoiceIssued {
			if strings.TrimSpace(sp.VoidOldReason) == "" {
				return Invoice{}, fmt.Errorf("store: invoice %s is not void: void it first, or give a reason to void it now", old.Number)
			}
			if err := voidTx(tx, old.Number, sp.VoidOldReason, inv.IssuedMS); err != nil {
				return Invoice{}, err
			}
		}
		inv.SupersedesID, inv.Supersedes = old.ID, old.Number
	}

	inv.Year = report.InvoiceYear(inv.IssuedMS, sp.Loc)
	if inv.Number == "" {
		var max sql.NullInt64
		if err := tx.QueryRow(`SELECT MAX(seq) FROM invoices WHERE year = ?`, inv.Year).Scan(&max); err != nil {
			return Invoice{}, fmt.Errorf("store: issue invoice: next number: %w", err)
		}
		inv.Number, inv.Seq = report.NextInvoiceNumber(inv.Year, int(max.Int64))
	} else {
		if err := checkOwnNumber(inv.Number); err != nil {
			return Invoice{}, err
		}
		inv.Seq = 0
	}
	var at int64
	switch err := tx.QueryRow(`SELECT issued_ms FROM invoices WHERE number = ?`, inv.Number).Scan(&at); {
	case err == nil:
		return Invoice{}, fmt.Errorf("store: invoice %s exists (issued %s)", inv.Number, time.UnixMilli(at).In(sp.Loc).Format("2006-01-02"))
	case !errors.Is(err, sql.ErrNoRows):
		return Invoice{}, fmt.Errorf("store: issue invoice: %w", err)
	}
	inv.Status, inv.VoidMS, inv.VoidReason = InvoiceIssued, 0, ""

	pdf, err := sp.Render(inv)
	if err != nil {
		return Invoice{}, fmt.Errorf("store: issue invoice: render: %w", err)
	}
	inv.PDF, inv.PDFSHA256 = pdf, hash(pdf)
	if inv.CanonicalJSON, err = canonicalJSON(inv); err != nil {
		return Invoice{}, err
	}
	inv.JSONSHA256 = hash(inv.CanonicalJSON)
	if inv.ID, err = insertInvoice(tx, inv); err != nil {
		return Invoice{}, err
	}
	if invoiceHook != nil {
		if err := invoiceHook("before-commit"); err != nil {
			return Invoice{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Invoice{}, fmt.Errorf("store: issue invoice: commit: %w", err)
	}
	return inv, nil
}

// checkOwnNumber refuses a chosen number that is empty, unfit for a file
// name, or in the default form, which the yearly sequence would reach.
func checkOwnNumber(n string) error {
	if len(n) > 64 {
		return fmt.Errorf("store: invoice number %q is longer than 64 characters", n)
	}
	for _, r := range n {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.') {
			return fmt.Errorf("store: invoice number %q: use letters, digits, - _ and . only", n)
		}
	}
	if n == "" || n[0] == '.' {
		return fmt.Errorf("store: invoice number %q", n)
	}
	if _, _, ok := report.ParseInvoiceNumber(n); ok {
		return fmt.Errorf("store: invoice number %q is in the YYYY-NNNN form the yearly sequence uses; leave it out to get the next one", n)
	}
	return nil
}

// VoidInvoice marks an issued invoice void, once, with a reason.
func (s *Store) VoidInvoice(number, reason string, atMS int64) (Invoice, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return Invoice{}, fmt.Errorf("store: void invoice: %w", err)
	}
	defer tx.Rollback()
	if err := voidTx(tx, number, reason, atMS); err != nil {
		return Invoice{}, err
	}
	if err := tx.Commit(); err != nil {
		return Invoice{}, fmt.Errorf("store: void invoice: %w", err)
	}
	return s.InvoiceByNumber(number)
}

func voidTx(tx *sql.Tx, number, reason string, atMS int64) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return errors.New("store: void invoice: a reason is required")
	}
	inv, err := invoiceSummary(tx, number)
	if err != nil {
		return fmt.Errorf("store: void %s: %w", number, err)
	}
	if inv.Status == InvoiceVoid {
		return fmt.Errorf("store: invoice %s is already void (%s)", inv.Number, inv.VoidReason)
	}
	if _, err := tx.Exec(`UPDATE invoices SET status = 'void', void_ms = ?, void_reason = ? WHERE id = ?`, atMS, reason, inv.ID); err != nil {
		return fmt.Errorf("store: void %s: %w", number, err)
	}
	return nil
}

// summaryCols are the columns every read takes; the document columns are
// read only for one invoice.
const summaryCols = `i.id, i.number, i.year, i.seq, i.status, i.issued_ms, i.supersedes_id, COALESCE(p.number, ''),
	i.void_ms, i.void_reason, i.issuer_name, i.issuer_address, i.issuer_tax_id,
	i.client_name, i.client_address, i.client_tax_id, i.project, i.period_from_ms, i.period_to_ms, i.tz,
	i.currency, i.minor_units, i.rate_minor, i.round_step_s, i.round_mode, i.tax_label, i.tax_bp, i.tax_note,
	i.seconds_billed, i.subtotal_minor, i.tax_minor, i.total_minor, i.note, i.generator, i.json_sha256, i.pdf_sha256`

const invoiceFrom = ` FROM invoices i LEFT JOIN invoices p ON p.id = i.supersedes_id`

type scanner interface{ Scan(...any) error }

func scanInvoice(r scanner, extra ...any) (Invoice, error) {
	var inv Invoice
	var seq, sup, voidMS sql.NullInt64
	var voidReason sql.NullString
	var issuerAddr, clientAddr string
	dst := []any{&inv.ID, &inv.Number, &inv.Year, &seq, &inv.Status, &inv.IssuedMS, &sup, &inv.Supersedes,
		&voidMS, &voidReason, &inv.IssuerName, &issuerAddr, &inv.IssuerTaxID,
		&inv.ClientName, &clientAddr, &inv.ClientTaxID, &inv.Project, &inv.PeriodFromMS, &inv.PeriodToMS, &inv.TZ,
		&inv.Currency, &inv.MinorUnits, &inv.RateMinor, &inv.RoundStepS, &inv.RoundMode, &inv.TaxLabel, &inv.TaxBP, &inv.TaxNote,
		&inv.SecondsBilled, &inv.SubtotalMinor, &inv.TaxMinor, &inv.TotalMinor, &inv.Note, &inv.Generator, &inv.JSONSHA256, &inv.PDFSHA256}
	if err := r.Scan(append(dst, extra...)...); err != nil {
		return Invoice{}, err
	}
	inv.Seq, inv.SupersedesID, inv.VoidMS, inv.VoidReason = int(seq.Int64), sup.Int64, voidMS.Int64, voidReason.String
	var err error
	if inv.IssuerAddress, err = decodeAddress(issuerAddr); err != nil {
		return Invoice{}, err
	}
	if inv.ClientAddress, err = decodeAddress(clientAddr); err != nil {
		return Invoice{}, err
	}
	return inv, nil
}

type querier interface {
	QueryRow(string, ...any) *sql.Row
}

func invoiceSummary(q querier, number string) (Invoice, error) {
	inv, err := scanInvoice(q.QueryRow(`SELECT `+summaryCols+invoiceFrom+` WHERE i.number = ?`, strings.TrimSpace(number)))
	if errors.Is(err, sql.ErrNoRows) {
		return Invoice{}, fmt.Errorf("%w: %s", ErrNoInvoice, number)
	}
	return inv, err
}

// InvoiceByNumber reads one invoice with its lines, items and document.
func (s *Store) InvoiceByNumber(number string) (Invoice, error) {
	var js string
	var pdf []byte
	inv, err := scanInvoice(s.db.QueryRow(`SELECT `+summaryCols+`, i.canonical_json, i.pdf`+invoiceFrom+` WHERE i.number = ?`,
		strings.TrimSpace(number)), &js, &pdf)
	if errors.Is(err, sql.ErrNoRows) {
		return Invoice{}, fmt.Errorf("%w: %s", ErrNoInvoice, number)
	}
	if err != nil {
		return Invoice{}, fmt.Errorf("store: invoice %s: %w", number, err)
	}
	inv.CanonicalJSON, inv.PDF = []byte(js), pdf
	rows, err := s.db.Query(`SELECT day_local, seconds_raw, seconds_billed, amount_minor FROM invoice_lines WHERE invoice_id = ? ORDER BY line_no`, inv.ID)
	if err != nil {
		return Invoice{}, fmt.Errorf("store: invoice %s lines: %w", number, err)
	}
	for rows.Next() {
		var l InvoiceLine
		if err := rows.Scan(&l.Day, &l.SecondsRaw, &l.SecondsBilled, &l.AmountMinor); err != nil {
			rows.Close()
			return Invoice{}, err
		}
		inv.Lines = append(inv.Lines, l)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return Invoice{}, err
	}
	rows, err = s.db.Query(`SELECT origin, origin_id, start_ms, end_ms, seconds, category, project, category_source, project_source
		FROM invoice_items WHERE invoice_id = ? ORDER BY rowid`, inv.ID)
	if err != nil {
		return Invoice{}, fmt.Errorf("store: invoice %s items: %w", number, err)
	}
	defer rows.Close()
	for rows.Next() {
		var it InvoiceItem
		if err := rows.Scan(&it.Origin, &it.OriginID, &it.StartMS, &it.EndMS, &it.Seconds, &it.Category, &it.Project, &it.CategorySource, &it.ProjectSource); err != nil {
			return Invoice{}, err
		}
		inv.Items = append(inv.Items, it)
	}
	return inv, rows.Err()
}

// Invoices lists the invoices of a year (0 = all) in issue order, without
// lines, items or document.
func (s *Store) Invoices(year int) ([]Invoice, error) {
	q := `SELECT ` + summaryCols + invoiceFrom
	var args []any
	if year != 0 {
		q += ` WHERE i.year = ?`
		args = append(args, year)
	}
	rows, err := s.db.Query(q+` ORDER BY i.issued_ms, i.id`, args...)
	if err != nil {
		return nil, fmt.Errorf("store: invoices: %w", err)
	}
	defer rows.Close()
	var out []Invoice
	for rows.Next() {
		inv, err := scanInvoice(rows)
		if err != nil {
			return nil, fmt.Errorf("store: invoices: %w", err)
		}
		out = append(out, inv)
	}
	return out, rows.Err()
}

// InvoiceNumbersIn lists the invoice numbers in the database file at path,
// read-only; none for a database from before invoices.
func InvoiceNumbersIn(path string) ([]string, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("store: invoice numbers: %w", err)
	}
	defer db.Close()
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'invoices'`).Scan(&n); err != nil {
		return nil, fmt.Errorf("store: invoice numbers: %w", err)
	}
	if n == 0 {
		return nil, nil
	}
	rows, err := db.Query(`SELECT number FROM invoices ORDER BY issued_ms, id`)
	if err != nil {
		return nil, fmt.Errorf("store: invoice numbers: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func insertInvoice(tx *sql.Tx, inv Invoice) (int64, error) {
	ia, _ := json.Marshal(nonNil(inv.IssuerAddress))
	ca, _ := json.Marshal(nonNil(inv.ClientAddress))
	res, err := tx.Exec(`INSERT INTO invoices(number, year, seq, status, issued_ms, supersedes_id,
		issuer_name, issuer_address, issuer_tax_id, client_name, client_address, client_tax_id,
		project, period_from_ms, period_to_ms, tz, currency, minor_units, rate_minor, round_step_s, round_mode,
		tax_label, tax_bp, tax_note, seconds_billed, subtotal_minor, tax_minor, total_minor, note, generator,
		canonical_json, json_sha256, pdf, pdf_sha256)
		VALUES(?, ?, ?, 'issued', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		inv.Number, inv.Year, nullInt(int64(inv.Seq)), inv.IssuedMS, nullInt(inv.SupersedesID),
		inv.IssuerName, string(ia), inv.IssuerTaxID, inv.ClientName, string(ca), inv.ClientTaxID,
		inv.Project, inv.PeriodFromMS, inv.PeriodToMS, inv.TZ, inv.Currency, inv.MinorUnits, inv.RateMinor, inv.RoundStepS, inv.RoundMode,
		inv.TaxLabel, inv.TaxBP, inv.TaxNote, inv.SecondsBilled, inv.SubtotalMinor, inv.TaxMinor, inv.TotalMinor, inv.Note, inv.Generator,
		string(inv.CanonicalJSON), inv.JSONSHA256, inv.PDF, inv.PDFSHA256)
	if err != nil {
		return 0, fmt.Errorf("store: insert invoice %s: %w", inv.Number, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	for i, l := range inv.Lines {
		if _, err := tx.Exec(`INSERT INTO invoice_lines(invoice_id, line_no, day_local, seconds_raw, seconds_billed, amount_minor) VALUES(?, ?, ?, ?, ?, ?)`,
			id, i+1, l.Day, l.SecondsRaw, l.SecondsBilled, l.AmountMinor); err != nil {
			return 0, fmt.Errorf("store: insert invoice %s line: %w", inv.Number, err)
		}
	}
	for _, it := range inv.Items {
		if _, err := tx.Exec(`INSERT INTO invoice_items(invoice_id, origin, origin_id, start_ms, end_ms, seconds, category, project, category_source, project_source)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, it.Origin, it.OriginID, it.StartMS, it.EndMS, it.Seconds, it.Category, it.Project, it.CategorySource, it.ProjectSource); err != nil {
			return 0, fmt.Errorf("store: insert invoice %s item: %w", inv.Number, err)
		}
	}
	return id, nil
}

func nullInt(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}

func nonNil(a []string) []string {
	if a == nil {
		return []string{}
	}
	return a
}

func decodeAddress(s string) ([]string, error) {
	var a []string
	if err := json.Unmarshal([]byte(s), &a); err != nil {
		return nil, fmt.Errorf("store: invoice address: %w", err)
	}
	if len(a) == 0 {
		return nil, nil
	}
	return a, nil
}

// normalizeInvoice copies d so the caller's slices are not shared, with
// empty slices as nil, as they read back.
func normalizeInvoice(d Invoice) Invoice {
	clone := func(a []string) []string {
		if len(a) == 0 {
			return nil
		}
		return append([]string(nil), a...)
	}
	d.IssuerAddress, d.ClientAddress = clone(d.IssuerAddress), clone(d.ClientAddress)
	if len(d.Lines) == 0 {
		d.Lines = nil
	} else {
		d.Lines = append([]InvoiceLine(nil), d.Lines...)
	}
	if len(d.Items) == 0 {
		d.Items = nil
	} else {
		d.Items = append([]InvoiceItem(nil), d.Items...)
	}
	d.ID, d.SupersedesID, d.Supersedes = 0, 0, ""
	return d
}

func hash(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// invoiceDoc is the canonical JSON of an issued invoice: every frozen
// field, the lines and items, and the PDF's hash, in a fixed key order. It
// leaves out the status and the void, which change, and the row ids.
type invoiceDoc struct {
	Format        int           `json:"format"`
	Number        string        `json:"number"`
	Year          int           `json:"year"`
	Seq           int           `json:"seq"`
	IssuedMS      int64         `json:"issued_ms"`
	Supersedes    string        `json:"supersedes"`
	Issuer        partyDoc      `json:"issuer"`
	Client        partyDoc      `json:"client"`
	Project       string        `json:"project"`
	PeriodFromMS  int64         `json:"period_from_ms"`
	PeriodToMS    int64         `json:"period_to_ms"`
	TZ            string        `json:"tz"`
	Currency      string        `json:"currency"`
	MinorUnits    int           `json:"minor_units"`
	RateMinor     int64         `json:"rate_minor"`
	RoundStepS    int64         `json:"round_step_s"`
	RoundMode     string        `json:"round_mode"`
	Tax           taxDoc        `json:"tax"`
	SecondsBilled int64         `json:"seconds_billed"`
	SubtotalMinor int64         `json:"subtotal_minor"`
	TaxMinor      int64         `json:"tax_minor"`
	TotalMinor    int64         `json:"total_minor"`
	Note          string        `json:"note"`
	Generator     string        `json:"generator"`
	PDFSHA256     string        `json:"pdf_sha256"`
	Lines         []InvoiceLine `json:"lines"`
	Items         []InvoiceItem `json:"items"`
}

type partyDoc struct {
	Name    string   `json:"name"`
	Address []string `json:"address"`
	TaxID   string   `json:"tax_id"`
}

type taxDoc struct {
	Label string `json:"label"`
	BP    int64  `json:"bp"`
	Note  string `json:"note"`
}

func docOf(inv Invoice) invoiceDoc {
	lines, items := inv.Lines, inv.Items
	if lines == nil {
		lines = []InvoiceLine{}
	}
	if items == nil {
		items = []InvoiceItem{}
	}
	return invoiceDoc{Format: invoiceFormat, Number: inv.Number, Year: inv.Year, Seq: inv.Seq, IssuedMS: inv.IssuedMS, Supersedes: inv.Supersedes,
		Issuer:  partyDoc{inv.IssuerName, nonNil(inv.IssuerAddress), inv.IssuerTaxID},
		Client:  partyDoc{inv.ClientName, nonNil(inv.ClientAddress), inv.ClientTaxID},
		Project: inv.Project, PeriodFromMS: inv.PeriodFromMS, PeriodToMS: inv.PeriodToMS, TZ: inv.TZ,
		Currency: inv.Currency, MinorUnits: inv.MinorUnits, RateMinor: inv.RateMinor, RoundStepS: inv.RoundStepS, RoundMode: inv.RoundMode,
		Tax:           taxDoc{inv.TaxLabel, inv.TaxBP, inv.TaxNote},
		SecondsBilled: inv.SecondsBilled, SubtotalMinor: inv.SubtotalMinor, TaxMinor: inv.TaxMinor, TotalMinor: inv.TotalMinor,
		Note: inv.Note, Generator: inv.Generator, PDFSHA256: inv.PDFSHA256, Lines: lines, Items: items}
}

// invoiceOfDoc is the invoice a canonical JSON describes, as issued.
func invoiceOfDoc(d invoiceDoc) Invoice {
	return normalizeInvoice(Invoice{Number: d.Number, Year: d.Year, Seq: d.Seq, Status: InvoiceIssued, IssuedMS: d.IssuedMS,
		IssuerName: d.Issuer.Name, IssuerAddress: d.Issuer.Address, IssuerTaxID: d.Issuer.TaxID,
		ClientName: d.Client.Name, ClientAddress: d.Client.Address, ClientTaxID: d.Client.TaxID,
		Project: d.Project, PeriodFromMS: d.PeriodFromMS, PeriodToMS: d.PeriodToMS, TZ: d.TZ,
		Currency: d.Currency, MinorUnits: d.MinorUnits, RateMinor: d.RateMinor, RoundStepS: d.RoundStepS, RoundMode: d.RoundMode,
		TaxLabel: d.Tax.Label, TaxBP: d.Tax.BP, TaxNote: d.Tax.Note,
		SecondsBilled: d.SecondsBilled, SubtotalMinor: d.SubtotalMinor, TaxMinor: d.TaxMinor, TotalMinor: d.TotalMinor,
		Note: d.Note, Generator: d.Generator, PDFSHA256: d.PDFSHA256, Lines: d.Lines, Items: d.Items})
}

func encodeCanonical(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, fmt.Errorf("store: canonical json: %w", err)
	}
	return b.Bytes(), nil
}

func canonicalJSON(inv Invoice) ([]byte, error) { return encodeCanonical(docOf(inv)) }

// BilledItem is an item of an issued (not void) invoice.
type BilledItem struct {
	Number         string
	Origin         string
	OriginID       int64
	StartMS, EndMS int64
}

// BilledItems lists the items of issued invoices that overlap [fromMS, toMS),
// in start order.
func (s *Store) BilledItems(fromMS, toMS int64) ([]BilledItem, error) {
	rows, err := s.db.Query(`SELECT i.number, it.origin, it.origin_id, it.start_ms, it.end_ms
		FROM invoice_items it JOIN invoices i ON i.id = it.invoice_id
		WHERE i.status = ? AND it.start_ms < ? AND it.end_ms > ?
		ORDER BY it.start_ms, i.number`, InvoiceIssued, toMS, fromMS)
	if err != nil {
		return nil, fmt.Errorf("store: billed items: %w", err)
	}
	defer rows.Close()
	var out []BilledItem
	for rows.Next() {
		var b BilledItem
		if err := rows.Scan(&b.Number, &b.Origin, &b.OriginID, &b.StartMS, &b.EndMS); err != nil {
			return nil, fmt.Errorf("store: billed items: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// BilledOn lists, in order and once each, the issued invoices holding an
// item of origin with one of ids.
func (s *Store) BilledOn(origin string, ids []int64) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	q := `SELECT DISTINCT i.number FROM invoice_items it JOIN invoices i ON i.id = it.invoice_id
		WHERE i.status = ? AND it.origin = ? AND it.origin_id IN (?` + strings.Repeat(",?", len(ids)-1) + `) ORDER BY i.number`
	args := []any{InvoiceIssued, origin}
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("store: billed on: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, fmt.Errorf("store: billed on: %w", err)
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

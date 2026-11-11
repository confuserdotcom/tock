package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"tock/internal/app"
	"tock/internal/report"
	"tock/internal/store"
)

const invoiceUsage = `invoice: one of
  tock invoice --project P [--from D] [--to D] [--client NAME] [-o FILE]   preview (DRAFT PDF, nothing stored)
  tock invoice issue --project P [...] [--number N] [--supersedes N [--void-old --reason R]] [--allow-overlap] [--yes]
  tock invoice void NUMBER --reason R
  tock invoice list [--year Y]
  tock invoice show NUMBER [--json | --pdf [-o FILE]]
  tock invoice check NUMBER      exit 3 when today's labels bill other time
  tock invoice verify [NUMBER]   exit 4 when a hash or file differs
  tock invoice recover           import invoice files the database lacks`

// exitStatus ends a command with code after printing msg ("" = nothing).
type exitStatus struct {
	code int
	msg  string
}

func (e exitStatus) Error() string { return e.msg }

// cmdInvoice previews, issues and keeps invoices (ADR 0001). Numbers and
// fields are sensible defaults, not any jurisdiction's invoicing rules.
func cmdInvoice(env Env, args []string) error {
	sub := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		sub, args = args[0], args[1:]
	}
	a, err := openApp(env)
	if err != nil {
		return err
	}
	defer a.Close()
	// Issued invoices whose files went missing (a crash after the commit,
	// a deleted folder) get them back on any invoice command but verify,
	// which reports them.
	if sub != "verify" {
		if fixed, err := a.RepairInvoiceFiles(); err != nil {
			fmt.Fprintf(env.Stderr, "tock: invoice files: %v\n", err)
		} else if len(fixed) > 0 {
			fmt.Fprintf(env.Stderr, "rewrote %d invoice file(s) from the database: %s\n", len(fixed), strings.Join(fixed, ", "))
		}
	}
	switch sub {
	case "":
		return invoicePreview(env, a, args)
	case "issue":
		return invoiceIssue(env, a, args)
	case "void":
		return invoiceVoid(env, a, args)
	case "list":
		return invoiceList(env, a, args)
	case "show":
		return invoiceShow(env, a, args)
	case "check":
		return invoiceCheck(env, a, args)
	case "verify":
		return invoiceVerify(env, a, args)
	case "recover":
		if len(args) > 0 {
			return usagef(invoiceUsage)
		}
		got, err := a.RecoverInvoices()
		if err != nil {
			return err
		}
		if len(got) == 0 {
			fmt.Fprintln(env.Stdout, "nothing to recover: every invoice file is in the database")
			return nil
		}
		fmt.Fprintf(env.Stdout, "recovered %d invoice(s): %s\n", len(got), strings.Join(got, ", "))
		return nil
	}
	return usagef(invoiceUsage)
}

// invoiceFlags are the flags preview and issue share.
type invoiceFlags struct {
	project, from, to, client, number, out *string
}

func newInvoiceFlags(fs *flag.FlagSet) invoiceFlags {
	return invoiceFlags{
		project: fs.String("project", "", "the project to bill (required; a unique prefix will do)"),
		from:    fs.String("from", "", "first day, YYYY-MM-DD (default: first day of last month)"),
		to:      fs.String("to", "", "last day, YYYY-MM-DD (default: last day of last month)"),
		client:  fs.String("client", "", "who the invoice is to (default: the project's client)"),
		number:  fs.String("number", "", "invoice number (default: the next YYYY-NNNN)"),
		out:     fs.String("o", "", "also write the PDF to this file"),
	}
}

func (f invoiceFlags) spec(env Env) (app.InvoiceSpec, error) {
	if *f.project == "" {
		return app.InvoiceSpec{}, usagef("invoice: --project is required")
	}
	now := env.Now()
	first := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	from, to := first.AddDate(0, -1, 0), first.AddDate(0, 0, -1)
	for _, d := range []struct {
		s   string
		dst *time.Time
	}{{*f.from, &from}, {*f.to, &to}} {
		if d.s == "" {
			continue
		}
		t, err := time.ParseInLocation("2006-01-02", d.s, now.Location())
		if err != nil {
			return app.InvoiceSpec{}, usagef("invoice: date %q, want YYYY-MM-DD", d.s)
		}
		*d.dst = t
	}
	if from.After(to) {
		return app.InvoiceSpec{}, usagef("invoice: --from is after --to")
	}
	return app.InvoiceSpec{Project: *f.project, From: from, To: to, Client: *f.client, Number: *f.number,
		Generator: "tock " + version}, nil
}

// printDraft prints a draft's totals and what to look at before issuing.
func printDraft(env Env, d app.InvoiceDraft) {
	r := d.Record
	fmt.Fprintf(env.Stdout, "%s, %s to %s: %d days, %s h", r.Project, d.Doc.PeriodFrom.Format("2006-01-02"),
		d.Doc.PeriodTo.Format("2006-01-02"), len(r.Lines), hoursText(r.SecondsBilled))
	if r.TaxBP > 0 {
		fmt.Fprintf(env.Stdout, ", %s + %s %s%%", money(r.SubtotalMinor, r), strings.TrimSpace(r.TaxLabel), report.FormatPercent(r.TaxBP))
	}
	fmt.Fprintf(env.Stdout, ", total %s\n", money(r.TotalMinor, r))
	if r.ClientName != "" {
		fmt.Fprintf(env.Stdout, "to %s\n", r.ClientName)
	}
	if strings.TrimSpace(r.IssuerName) == "" {
		fmt.Fprintln(env.Stdout, "your name is empty, so this can't be issued yet: [invoice] name in config.toml, or the dashboard's Settings")
	}
	if d.ModelOnlyS > 0 {
		fmt.Fprintf(env.Stdout, "%d min of it is on %s only because the model guessed so; check it in the day view before issuing\n",
			(d.ModelOnlyS+30)/60, r.Project)
	}
	if len(d.Overlaps) > 0 {
		fmt.Fprintf(env.Stdout, "invoice %s already bills %s over some of these days\n", strings.Join(d.Overlaps, ", "), r.Project)
	}
}

func hoursText(s int64) string { return strconv.FormatFloat(float64(s)/3600, 'f', 2, 64) }

// hoursPair formats two durations for a comparison: hours to a hundredth,
// or h:mm:ss when those would print the same for different durations.
func hoursPair(a, b int64) (string, string) {
	ha, hb := hoursText(a), hoursText(b)
	if ha == hb && a != b {
		return clockText(a), clockText(b)
	}
	return ha, hb
}

// clockText is s seconds as h:mm:ss.
func clockText(s int64) string {
	return fmt.Sprintf("%d:%02d:%02d", s/3600, s%3600/60, s%60)
}

// withUnit adds " h" to an hours figure from hoursPair.
func withUnit(s string) string {
	if strings.Contains(s, ":") {
		return s
	}
	return s + " h"
}

// issuedNowText is "issued A / now B" for check.
func issuedNowText(issued, now int64) string {
	a, b := hoursPair(issued, now)
	return "issued " + withUnit(a) + " / now " + withUnit(b)
}

func money(v int64, r store.Invoice) string {
	s := report.FormatMinor(v, r.MinorUnits)
	if r.Currency != "" {
		s += " " + r.Currency
	}
	return s
}

func invoicePreview(env Env, a *app.App, args []string) error {
	fs := newFlags("invoice")
	f := newInvoiceFlags(fs)
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return usagef("invoice: unexpected argument %q (subcommands: issue, void, list, show, check, verify, recover)", fs.Arg(0))
	}
	s, err := f.spec(env)
	if err != nil {
		return err
	}
	d, err := a.DraftInvoice(s)
	if err != nil {
		return err
	}
	b, err := app.DraftPDF(d)
	if err != nil {
		return err
	}
	path := *f.out
	if path == "" {
		if path, err = writeOwn(env, "drafts", "tock-draft-"+report.Slug(d.Record.Project)+".pdf", b); err != nil {
			return err
		}
	} else if err := os.WriteFile(path, b, 0o600); err != nil {
		return err
	}
	printDraft(env, d)
	fmt.Fprintf(env.Stdout, "draft: %s (nothing stored; tock invoice issue … issues it)\n", path)
	return nil
}

func invoiceIssue(env Env, a *app.App, args []string) error {
	fs := newFlags("invoice issue")
	f := newInvoiceFlags(fs)
	supersedes := fs.String("supersedes", "", "the number of the invoice this one replaces")
	voidOld := fs.Bool("void-old", false, "void the superseded invoice now (needs --reason)")
	reason := fs.String("reason", "", "why the superseded invoice is voided")
	overlap := fs.Bool("allow-overlap", false, "issue even if another invoice bills some of the same days")
	yes := fs.Bool("yes", false, "issue without asking about time only the model labelled")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return usagef("invoice issue: unexpected argument %q", fs.Arg(0))
	}
	s, err := f.spec(env)
	if err != nil {
		return err
	}
	if *voidOld != (*reason != "") || (*voidOld && *supersedes == "") {
		return usagef("invoice issue: --void-old and --reason go together, with --supersedes")
	}
	s.Supersedes, s.AllowOverlap = *supersedes, *overlap
	if *voidOld {
		s.VoidOldReason = *reason
	}
	d, err := a.DraftInvoice(s)
	if err != nil {
		return err
	}
	if strings.TrimSpace(d.Record.IssuerName) == "" {
		return app.ErrNoIssuer // before any question: confirming would only lead here
	}
	printDraft(env, d)
	if d.ModelOnlyS > 0 && !*yes {
		if !env.Interactive {
			return usagef("invoice issue: some billed time only the model labelled; review it, or confirm with --yes")
		}
		fmt.Fprint(env.Stdout, "Issue it anyway? [y/N] ")
		line, _ := bufio.NewReader(env.Stdin).ReadString('\n')
		if ans := strings.ToLower(strings.TrimSpace(line)); ans != "y" && ans != "yes" {
			fmt.Fprintln(env.Stdout, "nothing issued")
			return nil
		}
	}
	inv, err := a.IssueInvoice(s)
	if inv.Number == "" {
		return err
	}
	dir := store.InvoiceDir(env.dbPath())
	fmt.Fprintf(env.Stdout, "issued %s: %s\n", inv.Number, money(inv.TotalMinor, inv))
	if inv.Supersedes != "" {
		fmt.Fprintf(env.Stdout, "replaces %s, now void\n", inv.Supersedes)
	}
	fmt.Fprintf(env.Stdout, "%s\n%s\n", filepath.Join(dir, inv.Number+".pdf"), filepath.Join(dir, inv.Number+".json"))
	if err != nil {
		return fmt.Errorf("invoice %s is issued, but its files: %w (any tock invoice command writes them again)", inv.Number, err)
	}
	if *f.out != "" {
		return os.WriteFile(*f.out, inv.PDF, 0o600)
	}
	return nil
}

func invoiceVoid(env Env, a *app.App, args []string) error {
	fs := newFlags("invoice void")
	reason := fs.String("reason", "", "why (required)")
	if err := parseFlags(fs, moveFlagsFirst(args)); err != nil {
		return err
	}
	if fs.NArg() != 1 || strings.TrimSpace(*reason) == "" {
		return usagef("invoice void NUMBER --reason \"…\"")
	}
	inv, err := a.VoidInvoice(fs.Arg(0), *reason)
	if err != nil {
		return err
	}
	fmt.Fprintf(env.Stdout, "voided %s: %s\nit stays on record; its number is never used again\n", inv.Number, inv.VoidReason)
	return nil
}

func invoiceList(env Env, a *app.App, args []string) error {
	fs := newFlags("invoice list")
	year := fs.Int("year", 0, "only this year's invoices")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return usagef("invoice list [--year Y]")
	}
	list, err := a.Invoices(*year)
	if err != nil {
		return err
	}
	if len(list) == 0 {
		fmt.Fprintln(env.Stdout, "no invoices issued yet")
		return nil
	}
	loc := env.Now().Location()
	day := func(ms int64) string { return time.UnixMilli(ms).In(loc).Format("2006-01-02") }
	replacedBy := map[string]string{}
	for _, i := range list {
		if i.Supersedes != "" {
			replacedBy[i.Supersedes] = i.Number
		}
	}
	for _, i := range list {
		fmt.Fprintf(env.Stdout, "%s  %-6s  %s  %-14s  %-14s  %s to %s  %s\n", i.Number, i.Status, day(i.IssuedMS), i.Project, i.ClientName,
			day(i.PeriodFromMS), day(i.PeriodToMS-1), money(i.TotalMinor, i))
		if i.Status == store.InvoiceVoid {
			fmt.Fprintf(env.Stdout, "            void: %s\n", i.VoidReason)
		}
		if i.Supersedes != "" {
			fmt.Fprintf(env.Stdout, "            replaces %s\n", i.Supersedes)
		}
		if n := replacedBy[i.Number]; n != "" {
			fmt.Fprintf(env.Stdout, "            replaced by %s\n", n)
		}
	}
	return nil
}

func invoiceShow(env Env, a *app.App, args []string) error {
	fs := newFlags("invoice show")
	pdf := fs.Bool("pdf", false, "write the stored PDF instead of the summary")
	asJSON := fs.Bool("json", false, "print the invoice record exactly as stored, then its status")
	out := fs.String("o", "", "with --pdf: the file (default: stdout)")
	// NUMBER may come first: --pdf takes no value, so moveFlagsFirst
	// would not pair it right.
	number := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		number, args = args[0], args[1:]
	}
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if number == "" && fs.NArg() == 1 {
		number = fs.Arg(0)
	} else if fs.NArg() > 0 || number == "" {
		return usagef("invoice show NUMBER [--json | --pdf [-o FILE]]")
	}
	inv, err := a.InvoiceByNumber(number)
	if err != nil {
		return err
	}
	if *pdf && *asJSON {
		return usagef("invoice show: --json or --pdf, not both")
	}
	if *pdf {
		// The stored bytes, never a new rendering.
		if *out == "" {
			_, err := env.Stdout.Write(inv.PDF)
			return err
		}
		return os.WriteFile(*out, inv.PDF, 0o600)
	}
	if *out != "" {
		return usagef("invoice show: -o goes with --pdf")
	}
	if !*asJSON {
		printInvoice(env, inv)
		return nil
	}
	if _, err := env.Stdout.Write(inv.CanonicalJSON); err != nil {
		return err
	}
	status := map[string]any{"status": inv.Status}
	if inv.Status == store.InvoiceVoid {
		status["void_ms"], status["void_reason"] = inv.VoidMS, inv.VoidReason
	}
	b, _ := json.Marshal(status)
	fmt.Fprintf(env.Stdout, "%s\n", b)
	return nil
}

// printInvoice is the readable summary of an issued invoice; --json gives
// the stored record.
func printInvoice(env Env, inv store.Invoice) {
	w := env.Stdout
	// Dates in the zone the invoice was issued in, as its PDF prints them.
	loc, err := time.LoadLocation(inv.TZ)
	if err != nil {
		loc = env.Now().Location() // a zone with no IANA name: the days were this machine's
	}
	issued := time.UnixMilli(inv.IssuedMS).In(loc).Format("2006-01-02")
	fmt.Fprintf(w, "%s  %s %s", inv.Number, inv.Status, issued)
	if inv.Status == store.InvoiceVoid {
		fmt.Fprintf(w, ", void %s: %s", time.UnixMilli(inv.VoidMS).In(loc).Format("2006-01-02"), report.Safe(inv.VoidReason))
	}
	if inv.Supersedes != "" {
		fmt.Fprintf(w, ", replaces %s", inv.Supersedes)
	}
	fmt.Fprintln(w)
	from, to := inv.PeriodFromMS, inv.PeriodToMS-1
	fmt.Fprintf(w, "%s, %s to %s\n", report.Safe(inv.Project), time.UnixMilli(from).In(loc).Format("2006-01-02"),
		time.UnixMilli(to).In(loc).Format("2006-01-02"))
	var parties []string
	if inv.IssuerName != "" {
		parties = append(parties, "from "+report.Safe(inv.IssuerName))
	}
	if inv.ClientName != "" {
		parties = append(parties, "to "+report.Safe(inv.ClientName))
	}
	if len(parties) > 0 {
		fmt.Fprintln(w, strings.Join(parties, " "))
	}
	for _, l := range inv.Lines {
		fmt.Fprintf(w, "  %s  %s h  %s\n", l.Day, hoursText(l.SecondsBilled), money(l.AmountMinor, inv))
	}
	fmt.Fprintf(w, "%s h at rate %s/h", hoursText(inv.SecondsBilled), money(inv.RateMinor, inv))
	if inv.TaxBP > 0 {
		fmt.Fprintf(w, ", subtotal %s, %s %s%% %s", money(inv.SubtotalMinor, inv), report.Safe(strings.TrimSpace(inv.TaxLabel)),
			report.FormatPercent(inv.TaxBP), money(inv.TaxMinor, inv))
	}
	fmt.Fprintf(w, ", total %s\n", money(inv.TotalMinor, inv))
	fmt.Fprintf(w, "pdf sha256 %s (tock invoice show %s --pdf -o FILE)\n", inv.PDFSHA256, inv.Number)
}

func invoiceCheck(env Env, a *app.App, args []string) error {
	if len(args) != 1 {
		return usagef("invoice check NUMBER")
	}
	c, err := a.CheckInvoice(args[0])
	if err != nil {
		return err
	}
	fmt.Fprintf(env.Stdout, "%s: %s\n", c.Invoice.Number, issuedNowText(c.IssuedS, c.NowS))
	if !c.Drift() {
		fmt.Fprintln(env.Stdout, "today's labels bill the same time")
		return nil
	}
	for _, d := range c.Days {
		fmt.Fprintf(env.Stdout, "  %s  %s\n", d.Day, issuedNowText(d.IssuedS, d.NowS))
	}
	fmt.Fprintln(env.Stdout, "the invoice stays as issued; void it and issue a new one with --supersedes to bill the change")
	return exitStatus{code: 3}
}

func invoiceVerify(env Env, a *app.App, args []string) error {
	if len(args) > 1 {
		return usagef("invoice verify [NUMBER]")
	}
	number := ""
	if len(args) == 1 {
		number = args[0]
	}
	checks, err := a.VerifyInvoices(number)
	if err != nil {
		return err
	}
	bad := 0
	for _, c := range checks {
		if c.OK() {
			fmt.Fprintf(env.Stdout, "%s  ok\n", c.Number)
			continue
		}
		bad++
		fmt.Fprintf(env.Stdout, "%s  %s\n", c.Number, strings.Join(c.Problems, "; "))
	}
	if len(checks) == 0 {
		fmt.Fprintln(env.Stdout, "no invoices issued yet")
	}
	if bad > 0 {
		return exitStatus{code: 4, msg: fmt.Sprintf("%d invoice(s) differ from their record; tock invoice show NUMBER --pdf -o FILE writes the stored PDF", bad)}
	}
	return nil
}

package main

import (
	"fmt"
	"strings"

	"tock/internal/store"
)

// doctorInvoices writes the files of issued invoices that went missing and
// checks the rest against their record. Silent with no invoices.
func doctorInvoices(env Env, ok func(name, detail string), bad func(name, detail, fix string)) {
	st, err := openStore(env)
	if err != nil {
		return
	}
	defer st.Close()
	list, err := st.Invoices(0)
	if err != nil {
		bad("invoices", err.Error(), "tock invoice verify")
		return
	}
	if len(list) == 0 {
		return
	}
	dir := store.InvoiceDir(env.dbPath())
	fixed, err := st.RepairInvoiceFiles(dir)
	if err != nil {
		bad("invoices", "files: "+err.Error(), "check "+dir)
		return
	}
	checks, err := st.VerifyInvoices(dir, "")
	if err != nil {
		bad("invoices", err.Error(), "tock invoice verify")
		return
	}
	var differ []string
	for _, c := range checks {
		if !c.OK() {
			differ = append(differ, c.Number)
		}
	}
	if len(differ) > 0 {
		bad("invoices", strings.Join(differ, ", ")+" differ from their record", "tock invoice verify")
		return
	}
	detail := plural(len(list), "invoice") + ", files match"
	if len(fixed) > 0 {
		detail += fmt.Sprintf(" (rewrote %s)", strings.Join(fixed, ", "))
	}
	ok("invoices", detail)
}

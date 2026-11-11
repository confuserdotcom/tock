-- Issued invoices. An invoice is a snapshot: every field the
-- document shows is copied here at issue and never read again from the
-- config or the rules. Labels stay live, and tock invoice check shows
-- drift.
--
-- Triggers keep issued rows as issued: the only change allowed is
-- status issued -> void with a reason and a time, once. Nothing is ever
-- deleted. The triggers guard against tock's own code and anything else
-- using SQL. They do not guard against editing the file, DROP TRIGGER,
-- restoring an older backup or disk damage (docs/stability.md).
--
-- No titles or URLs are stored, so prune and the privacy settings never
-- touch billing records. Local to this machine: never synced.
CREATE TABLE invoices (
    id             INTEGER PRIMARY KEY,
    number         TEXT NOT NULL UNIQUE,
    year           INTEGER NOT NULL,
    seq            INTEGER,
    status         TEXT NOT NULL DEFAULT 'issued' CHECK (status IN ('issued', 'void')),
    issued_ms      INTEGER NOT NULL,
    supersedes_id  INTEGER REFERENCES invoices(id),
    void_ms        INTEGER,
    void_reason    TEXT,
    issuer_name    TEXT NOT NULL DEFAULT '',
    issuer_address TEXT NOT NULL DEFAULT '[]',
    issuer_tax_id  TEXT NOT NULL DEFAULT '',
    client_name    TEXT NOT NULL DEFAULT '',
    client_address TEXT NOT NULL DEFAULT '[]',
    client_tax_id  TEXT NOT NULL DEFAULT '',
    project        TEXT NOT NULL,
    period_from_ms INTEGER NOT NULL,
    period_to_ms   INTEGER NOT NULL,
    tz             TEXT NOT NULL,
    currency       TEXT NOT NULL DEFAULT '',
    minor_units    INTEGER NOT NULL,
    rate_minor     INTEGER NOT NULL,
    round_step_s   INTEGER NOT NULL,
    round_mode     TEXT NOT NULL,
    tax_label      TEXT NOT NULL DEFAULT '',
    tax_bp         INTEGER NOT NULL DEFAULT 0,
    tax_note       TEXT NOT NULL DEFAULT '',
    seconds_billed INTEGER NOT NULL,
    subtotal_minor INTEGER NOT NULL,
    tax_minor      INTEGER NOT NULL,
    total_minor    INTEGER NOT NULL,
    note           TEXT NOT NULL DEFAULT '',
    generator      TEXT NOT NULL DEFAULT '',
    canonical_json TEXT NOT NULL,
    json_sha256    TEXT NOT NULL,
    pdf            BLOB NOT NULL,
    pdf_sha256     TEXT NOT NULL,
    UNIQUE (year, seq)
);
CREATE INDEX invoices_project ON invoices(project, period_from_ms);

-- The rows printed on the document: one per local day.
CREATE TABLE invoice_lines (
    invoice_id     INTEGER NOT NULL REFERENCES invoices(id),
    line_no        INTEGER NOT NULL,
    day_local      TEXT NOT NULL,
    seconds_raw    INTEGER NOT NULL,
    seconds_billed INTEGER NOT NULL,
    amount_minor   INTEGER NOT NULL,
    PRIMARY KEY (invoice_id, line_no)
);

-- The billed time behind the lines, for audit and drift checks: a span of
-- this machine, a timeline edit, away time, a calendar meeting or another
-- machine's span (origin_id is that row's id).
CREATE TABLE invoice_items (
    invoice_id      INTEGER NOT NULL REFERENCES invoices(id),
    origin          TEXT NOT NULL CHECK (origin IN ('span', 'edit', 'away', 'meeting', 'remote')),
    origin_id       INTEGER NOT NULL,
    start_ms        INTEGER NOT NULL,
    end_ms          INTEGER NOT NULL,
    seconds         INTEGER NOT NULL,
    category        TEXT NOT NULL DEFAULT '',
    project         TEXT NOT NULL DEFAULT '',
    category_source TEXT NOT NULL DEFAULT '',
    project_source  TEXT NOT NULL DEFAULT ''
);
CREATE INDEX invoice_items_invoice ON invoice_items(invoice_id);
CREATE INDEX invoice_items_start ON invoice_items(start_ms);

CREATE TRIGGER invoices_frozen BEFORE UPDATE ON invoices
WHEN NOT (
    OLD.status = 'issued' AND NEW.status = 'void'
    AND NEW.void_ms IS NOT NULL
    AND NEW.void_reason IS NOT NULL AND trim(NEW.void_reason) <> ''
    AND NEW.id IS OLD.id AND NEW.number IS OLD.number
    AND NEW.year IS OLD.year AND NEW.seq IS OLD.seq
    AND NEW.issued_ms IS OLD.issued_ms AND NEW.supersedes_id IS OLD.supersedes_id
    AND NEW.issuer_name IS OLD.issuer_name AND NEW.issuer_address IS OLD.issuer_address
    AND NEW.issuer_tax_id IS OLD.issuer_tax_id
    AND NEW.client_name IS OLD.client_name AND NEW.client_address IS OLD.client_address
    AND NEW.client_tax_id IS OLD.client_tax_id
    AND NEW.project IS OLD.project AND NEW.period_from_ms IS OLD.period_from_ms
    AND NEW.period_to_ms IS OLD.period_to_ms AND NEW.tz IS OLD.tz
    AND NEW.currency IS OLD.currency AND NEW.minor_units IS OLD.minor_units
    AND NEW.rate_minor IS OLD.rate_minor AND NEW.round_step_s IS OLD.round_step_s
    AND NEW.round_mode IS OLD.round_mode
    AND NEW.tax_label IS OLD.tax_label AND NEW.tax_bp IS OLD.tax_bp AND NEW.tax_note IS OLD.tax_note
    AND NEW.seconds_billed IS OLD.seconds_billed AND NEW.subtotal_minor IS OLD.subtotal_minor
    AND NEW.tax_minor IS OLD.tax_minor AND NEW.total_minor IS OLD.total_minor
    AND NEW.note IS OLD.note AND NEW.generator IS OLD.generator
    AND NEW.canonical_json IS OLD.canonical_json AND NEW.json_sha256 IS OLD.json_sha256
    AND NEW.pdf IS OLD.pdf AND NEW.pdf_sha256 IS OLD.pdf_sha256
)
BEGIN
    SELECT RAISE(ABORT, 'invoices: an issued invoice cannot be changed, only voided once with a reason');
END;

CREATE TRIGGER invoices_no_delete BEFORE DELETE ON invoices
BEGIN
    SELECT RAISE(ABORT, 'invoices: invoices are never deleted');
END;

CREATE TRIGGER invoice_lines_frozen BEFORE UPDATE ON invoice_lines
BEGIN
    SELECT RAISE(ABORT, 'invoices: invoice lines cannot be changed');
END;

CREATE TRIGGER invoice_lines_no_delete BEFORE DELETE ON invoice_lines
BEGIN
    SELECT RAISE(ABORT, 'invoices: invoice lines are never deleted');
END;

CREATE TRIGGER invoice_items_frozen BEFORE UPDATE ON invoice_items
BEGIN
    SELECT RAISE(ABORT, 'invoices: invoice items cannot be changed');
END;

CREATE TRIGGER invoice_items_no_delete BEFORE DELETE ON invoice_items
BEGIN
    SELECT RAISE(ABORT, 'invoices: invoice items are never deleted');
END;

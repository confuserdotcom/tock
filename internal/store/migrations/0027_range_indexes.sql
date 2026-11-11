-- Range reads (Spans, RemoteViews) bound start from below by the longest
-- row, so a day read scans that day's rows instead of all history before
-- it. These indexes on the row length make max(length) one index lookup.
CREATE INDEX span_len ON span("end" - start);
CREATE INDEX remote_view_len ON remote_view(end_ms - start_ms);

-- Per-administrator counter for the supplier payment PDF: each export bumps it and the number
-- goes into the file name, so a stack of monthly reports for one administrator stays ordered.
-- Additive with a default, so it fills existing rows without blocking the API boot in production.
ALTER TABLE people ADD COLUMN report_export_counter INTEGER NOT NULL DEFAULT 0;

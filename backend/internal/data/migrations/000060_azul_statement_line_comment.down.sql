-- Reverses 000060. Drops the stored comment. Nothing settled derives from it - it only feeds the
-- statement page's promotion labelling - so what is lost is that distinction, not any figure.
ALTER TABLE azul_statement_lines DROP COLUMN IF EXISTS comment;

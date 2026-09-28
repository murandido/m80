-- Reverses 000067. Drops the stored position. Nothing settled derives from it - it only orders the
-- statement page within a day - so what is lost is that order, not any figure.
ALTER TABLE azul_statement_lines DROP COLUMN IF EXISTS position_in_day;

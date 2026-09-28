-- Keep where each synced line sat within its day. The airline gives day precision only, so rows of
-- one day used to tie-break on their random uuid, which the window replacement regenerates on every
-- sync - the day's movements reshuffled each round. The legacy endpoint does return each day newest
-- first (checked against the issuances' own clocks), so that position is the only real order there
-- is. Additive and nullable: old rows read NULL and sort after the positioned ones until re-synced.
ALTER TABLE azul_statement_lines ADD COLUMN position_in_day INTEGER NULL;

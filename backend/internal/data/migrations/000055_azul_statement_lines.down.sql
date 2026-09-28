-- Reverses 000055. Drops the synced statement cache. It is a cache of what the airline's API
-- can be asked for again, so what is lost is re-fetchable, not a settled figure.
DROP TABLE IF EXISTS azul_statement_lines;

-- Keep the airline's free-text comment on each synced line. Until now only the locator and route
-- were parsed out of it and the rest dropped; the statement page needs the wording itself to tell
-- a promotion apart from an ordinary correction or transfer ("Liberalidade - promoção", "crédito
-- de pontos em campanha bonificada"), which the type code alone does not say. Additive and
-- nullable, so this migration changes no existing number and old rows read NULL until re-synced.
ALTER TABLE azul_statement_lines ADD COLUMN comment TEXT NULL;

-- Indexes for the columns the panel actually filters, joins and preloads on.
--
-- Postgres indexes a primary key and a unique constraint automatically, but never
-- the referencing side of a foreign key. Every column below was therefore a
-- sequential scan on every panel load: the issuances grid opens filtered on
-- "today + me" (issuer_id + created_at), the sales screen joins issuances to
-- accounts, programs, users and clients, and every listing Preloads its nicknames
-- by parent id. None of that is a query we can write differently - it is missing
-- index, and the tables only grow.
--
-- CREATE INDEX (not CONCURRENTLY) on purpose: migrations run inside the
-- transaction golang-migrate opens at API boot, where CONCURRENTLY is not
-- allowed. It takes an exclusive lock for the duration of the build, which on
-- tables this size is milliseconds - and the API is not serving yet anyway.

-- The issuances grid: its default filter, its dimension filters, and the
-- cancellation branch of the same filter (see applyIssuanceListFilters).
CREATE INDEX idx_issuances_created_at         ON issuances (created_at);
CREATE INDEX idx_issuances_cancelled_at       ON issuances (cancelled_at) WHERE cancelled_at IS NOT NULL;
CREATE INDEX idx_issuances_issuer_id          ON issuances (issuer_id);
CREATE INDEX idx_issuances_cancelled_by_id    ON issuances (cancelled_by_id);
CREATE INDEX idx_issuances_account_id         ON issuances (account_id);
CREATE INDEX idx_issuances_loyalty_program_id ON issuances (loyalty_program_id);
CREATE INDEX idx_issuances_airline_id         ON issuances (airline_id);
CREATE INDEX idx_issuances_agency_id          ON issuances (agency_id);
CREATE INDEX idx_issuances_account_nickname_id ON issuances (account_nickname_id);

-- Nickname lookups. Both the Preload and the EXISTS clause behind the search box
-- reach these tables by parent id, once per listing.
CREATE INDEX idx_account_nicknames_account_id ON account_nicknames (account_id);
CREATE INDEX idx_agency_nicknames_agency_id   ON agency_nicknames (agency_id);
CREATE INDEX idx_person_nicknames_person_id   ON person_nicknames (person_id);

-- The accounts listing filters by program and joins the holder; the supplier
-- report joins the administrator as well.
CREATE INDEX idx_accounts_loyalty_program_id ON accounts (loyalty_program_id);
CREATE INDEX idx_accounts_holder_id          ON accounts (holder_id);
CREATE INDEX idx_accounts_administrator_id   ON accounts (administrator_id);

-- The sales listing LEFT JOINs the client of every row.
CREATE INDEX idx_sales_client_id ON sales (client_id);

-- The supplier report joins each program to its default airline.
CREATE INDEX idx_loyalty_programs_default_airline_id ON loyalty_programs (default_airline_id);

-- account.replaceBeneficiaryUses reads and rewrites the uses of one issuance on
-- every issuance edit that touches passengers.
CREATE INDEX idx_account_beneficiary_uses_issuance_id ON account_beneficiary_uses (issuance_id);

-- Migration 45 made this an ON DELETE CASCADE, and Postgres checks the child
-- table on every parent delete - which without this is a scan of the whole
-- enrollment history.
CREATE INDEX idx_promo_enrollments_document_id ON promo_enrollments (document_id);

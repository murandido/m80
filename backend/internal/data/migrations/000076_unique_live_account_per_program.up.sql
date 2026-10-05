-- One live account per loyalty program for each holder, said by the database. The API and the
-- bot already keep to it (internal/account/programs.go); production was checked read-only
-- before this was written and no holder had two live accounts in one program.
--
-- Partial on purpose: an archived account keeps its holder and program for history, and
-- archiving the old account is exactly how an operator makes room for a new one.
CREATE UNIQUE INDEX idx_accounts_one_live_per_holder_program
    ON accounts (holder_id, loyalty_program_id)
    WHERE deleted_at IS NULL;

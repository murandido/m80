-- Pulling an issuance from Traveos can run for minutes - the vendor drives the
-- airline's own website - but the only trace of it lived in the browser that
-- started it. Leaving the screen lost the spinner and the row lock, and nobody
-- else ever saw that the row was busy.
--
-- traveos_syncing_at is that trace, on the row itself: set when the pull starts
-- (whether it came from the panel or from the bot) and cleared when it ends,
-- including on failure. A marker older than the sync's own 3-minute timeout is
-- read as stale, so a process that died mid-pull does not lock a row forever.
ALTER TABLE issuances ADD COLUMN traveos_syncing_at TIMESTAMPTZ NULL;

-- Partial: the interesting rows are the handful being pulled right now, never
-- the whole table.
CREATE INDEX idx_issuances_traveos_syncing_at ON issuances (traveos_syncing_at)
    WHERE traveos_syncing_at IS NOT NULL;

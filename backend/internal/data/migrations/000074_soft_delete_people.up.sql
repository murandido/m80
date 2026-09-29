-- People can be archived, the way accounts already are (migration 29). A holder the bot
-- created from a mistyped message ("119,08") cannot be deleted for real while an archived
-- account still names them - accounts.holder_id is ON DELETE RESTRICT - so archiving is how
-- they leave the panel. Additive: every existing row stays live.
ALTER TABLE people ADD COLUMN deleted_at TIMESTAMPTZ NULL;
CREATE INDEX idx_people_deleted_at ON people (deleted_at);

-- Which groups may trigger a promotion enrolment. Off by default: a bare link in an
-- issuance group must stay a bare link, and turning this on is what makes it a command.
--
-- Same shape as pdf_mode in migration 27, and the same warning applies - the Go model
-- must declare this column WITHOUT a `default` tag, or GORM treats an explicit false as
-- "unset" and lets this DEFAULT fill it back in.
ALTER TABLE listened_chats
    ADD COLUMN promo_mode BOOLEAN NOT NULL DEFAULT FALSE;

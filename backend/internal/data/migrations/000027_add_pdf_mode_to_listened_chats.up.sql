-- pdf_mode turns a monitored chat into one that also receives the ticket as a
-- PDF after every EMISSAO the bot completes. Per chat and not global: a group
-- that talks to clients wants the file, an internal operations group does not.
--
-- Defaults to false so enabling it is always somebody's decision - the bot
-- posting documents into a group nobody asked would be its own kind of failure.
ALTER TABLE listened_chats
    ADD COLUMN pdf_mode BOOLEAN NOT NULL DEFAULT FALSE;

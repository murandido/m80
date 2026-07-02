CREATE TYPE chat_status AS ENUM ('pending', 'approved', 'denied');

CREATE TABLE listened_chats (
    id               UUID                     NOT NULL DEFAULT gen_random_uuid(),
    chat_jid         VARCHAR                  NOT NULL,
    friendly_name    VARCHAR                  NULL,
    requested_by_id  UUID                     NOT NULL,
    approved_by_id   UUID                     NULL,
    status           chat_status              NOT NULL DEFAULT 'pending',
    active           BOOLEAN                  NOT NULL DEFAULT TRUE,
    created_at       TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT listened_chats_pkey PRIMARY KEY (id),
    CONSTRAINT listened_chats_chat_jid_unique UNIQUE (chat_jid),
    CONSTRAINT listened_chats_requested_by_fk
        FOREIGN KEY (requested_by_id) REFERENCES users(id) ON DELETE RESTRICT,
    CONSTRAINT listened_chats_approved_by_fk
        FOREIGN KEY (approved_by_id) REFERENCES users(id) ON DELETE SET NULL
);

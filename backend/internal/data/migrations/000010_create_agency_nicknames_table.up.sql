DROP TABLE IF EXISTS agency_nicknames;

CREATE TABLE agency_nicknames (
    id         UUID                     NOT NULL DEFAULT gen_random_uuid(),
    agency_id  UUID                     NOT NULL,
    nickname   VARCHAR                  NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT agency_nicknames_pkey PRIMARY KEY (id),
    CONSTRAINT agency_nicknames_agency_fk
        FOREIGN KEY (agency_id) REFERENCES agencies(id) ON DELETE CASCADE,
    CONSTRAINT agency_nicknames_nickname_unique UNIQUE (nickname)
);

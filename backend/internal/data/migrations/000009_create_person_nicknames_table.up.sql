DROP TABLE IF EXISTS person_nicknames;

CREATE TABLE person_nicknames (
    id         UUID                     NOT NULL DEFAULT gen_random_uuid(),
    person_id  UUID                     NOT NULL,
    nickname   VARCHAR                  NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT person_nicknames_pkey PRIMARY KEY (id),
    CONSTRAINT person_nicknames_person_fk
        FOREIGN KEY (person_id) REFERENCES people(id) ON DELETE CASCADE,
    CONSTRAINT person_nicknames_nickname_unique UNIQUE (nickname)
);

DROP TABLE IF EXISTS observation_presets;

CREATE TABLE observation_presets (
    id         UUID                     NOT NULL DEFAULT gen_random_uuid(),
    code       VARCHAR                  NOT NULL,
    text       VARCHAR                  NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT observation_presets_pkey PRIMARY KEY (id),
    CONSTRAINT observation_presets_code_unique UNIQUE (code)
);

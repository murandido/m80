DROP TABLE IF EXISTS users;
DROP TYPE IF EXISTS user_role;

CREATE TYPE user_role AS ENUM ('user', 'admin');

CREATE TABLE users (
    id         UUID                     NOT NULL DEFAULT gen_random_uuid(),
    name       VARCHAR                  NOT NULL,
    phone      VARCHAR                  NOT NULL,
    password   VARCHAR                  NOT NULL,
    role       user_role                NOT NULL DEFAULT 'user',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE NULL,
    CONSTRAINT users_pkey PRIMARY KEY (id)
);

-- Partial unique: phone reutilizável após soft-delete
CREATE UNIQUE INDEX idx_users_phone ON users (phone) WHERE deleted_at IS NULL;

-- Index para filtros de soft-delete
CREATE INDEX idx_users_deleted_at ON users (deleted_at);

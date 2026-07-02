CREATE TYPE audit_action AS ENUM ('created', 'updated', 'cancelled', 'restored', 'deleted');

CREATE TABLE audit_logs (
    id             UUID                     NOT NULL DEFAULT gen_random_uuid(),
    issuance_id    UUID                     NOT NULL,
    changed_by_id  UUID                     NOT NULL,
    action         audit_action             NOT NULL,
    details        JSONB                    NULL,
    created_at     TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT audit_logs_pkey PRIMARY KEY (id),
    CONSTRAINT audit_logs_issuance_fk
        FOREIGN KEY (issuance_id) REFERENCES issuances(id) ON DELETE RESTRICT,
    CONSTRAINT audit_logs_changed_by_fk
        FOREIGN KEY (changed_by_id) REFERENCES users(id) ON DELETE RESTRICT
);

CREATE INDEX idx_audit_logs_issuance_id ON audit_logs (issuance_id);

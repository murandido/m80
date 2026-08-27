-- Automated enrolment in loyalty transfer-bonus promotions.
--
-- The agency keeps its own accounts stocked with points and buys points whenever a
-- transfer bonus runs. Nearly every campaign requires enrolling a CPF *before*
-- transferring; transferring without it forfeits the bonus. The windows last about two
-- days, and forgetting is a direct loss. These three tables let the bot do the
-- enrolling when somebody drops the campaign link in an authorised group.

-- How to drive one site's enrolment form.
--
-- This lives in the database rather than in code because of the two-day windows: when a
-- campaign changes its markup, the fix has to be minutes away, not a branch, a review
-- and a deploy label. Every selector is nullable - NULL means "work this piece out
-- heuristically", so a site that behaves conventionally needs no selectors at all and a
-- site that does not can be pinned down one field at a time.
CREATE TABLE promo_recipes (
    id                UUID                     NOT NULL DEFAULT gen_random_uuid(),
    -- Registrable domain, without scheme or www: 'voeazul.com.br'. It is how an
    -- incoming link finds its recipe, so one row per site.
    domain            TEXT                     NOT NULL,
    name              TEXT                     NOT NULL,
    -- Which pool of CPFs this site enrols. A document is registered per programme, so
    -- the same person can hold both an Azul and a Smiles account.
    loyalty_program_id UUID                    NOT NULL,
    cpf_selector      TEXT                     NULL,
    checkbox_selector TEXT                     NULL,
    submit_selector   TEXT                     NULL,
    -- Regular expressions matched against the text the site shows after submitting.
    -- NULL falls back to the wording built into the service.
    success_pattern   TEXT                     NULL,
    failure_pattern   TEXT                     NULL,
    -- How long to let the page settle before reading the form. Single-page apps render
    -- the promotion widget after the first paint.
    wait_seconds      INTEGER                  NOT NULL DEFAULT 5,
    active            BOOLEAN                  NOT NULL DEFAULT TRUE,
    notes             TEXT                     NULL,
    created_at        TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT promo_recipes_pkey PRIMARY KEY (id),
    CONSTRAINT promo_recipes_domain_unique UNIQUE (domain),
    CONSTRAINT promo_recipes_wait_seconds_sane CHECK (wait_seconds BETWEEN 0 AND 120),
    CONSTRAINT promo_recipes_program_fk
        FOREIGN KEY (loyalty_program_id) REFERENCES loyalty_programs(id) ON DELETE RESTRICT
);

-- The CPFs the agency enrols, one row per document per programme.
--
-- Encrypted at rest with the same AES-256-GCM helper as people.cpf and
-- accounts.password, and paired with a blind index for the reason spelled out in
-- migration 38: the GCM nonce is random, so ciphertext differs on every write and a
-- unique index over the encrypted column would never catch a duplicate.
CREATE TABLE loyalty_documents (
    id                 UUID                     NOT NULL DEFAULT gen_random_uuid(),
    -- Whose document it is, for the panel and for the message the group gets back.
    name               TEXT                     NOT NULL,
    cpf                TEXT                     NOT NULL,
    cpf_fingerprint    TEXT                     NOT NULL,
    loyalty_program_id UUID                     NOT NULL,
    -- Retired documents stay on the table: their enrolment history has to survive, and
    -- promo_enrollments references them.
    active             BOOLEAN                  NOT NULL DEFAULT TRUE,
    created_at         TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT loyalty_documents_pkey PRIMARY KEY (id),
    -- Per programme, not globally: the same CPF legitimately belongs to an Azul pool
    -- and a Smiles pool at once.
    CONSTRAINT loyalty_documents_program_cpf_unique UNIQUE (loyalty_program_id, cpf_fingerprint),
    CONSTRAINT loyalty_documents_program_fk
        FOREIGN KEY (loyalty_program_id) REFERENCES loyalty_programs(id) ON DELETE RESTRICT
);

CREATE INDEX idx_loyalty_documents_program ON loyalty_documents (loyalty_program_id);

-- What was attempted, for whom, and how it went.
--
-- This table is the only source of truth about who is already enrolled, and that is a
-- deliberate design decision rather than a convenience. The Azul campaign we tested
-- against accepts the same CPF over and over and answers "Solicitação enviada" every
-- time, so the site itself cannot tell us whether a document is already in. Other
-- campaigns refuse, or warn, or say nothing at all - the behaviour varies per campaign,
-- so building on it would build on sand.
--
-- The screenshot is not stored. It goes to the group as proof and is discarded: images
-- in a column would bloat the nightly dump for no lasting gain.
CREATE TABLE promo_enrollments (
    id            UUID                     NOT NULL DEFAULT gen_random_uuid(),
    -- The link exactly as it arrived, kept for the audit trail.
    url           TEXT                     NOT NULL,
    -- The link canonicalised to scheme + host + path, with the query string dropped.
    -- Promotion links circulate with utm_* and other tracking parameters attached, so
    -- without this the same campaign pasted twice reads as two different campaigns and
    -- everybody gets enrolled again.
    url_key       TEXT                     NOT NULL,
    document_id   UUID                     NOT NULL,
    -- Which recipe drove it. Nullable, and ON DELETE SET NULL: deleting a recipe must
    -- not erase the history of what it did.
    recipe_id     UUID                     NULL,
    -- success | failure | unknown | refused | error - see internal/promotion.
    status        TEXT                     NOT NULL,
    message       TEXT                     NULL,
    -- Who dropped the link, and where. Both nullable: an enrolment triggered from the
    -- panel has no chat behind it.
    created_by_id UUID                     NULL,
    chat_jid      TEXT                     NULL,
    created_at    TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT promo_enrollments_pkey PRIMARY KEY (id),
    CONSTRAINT promo_enrollments_document_fk
        FOREIGN KEY (document_id) REFERENCES loyalty_documents(id) ON DELETE RESTRICT,
    CONSTRAINT promo_enrollments_recipe_fk
        FOREIGN KEY (recipe_id) REFERENCES promo_recipes(id) ON DELETE SET NULL,
    CONSTRAINT promo_enrollments_created_by_fk
        FOREIGN KEY (created_by_id) REFERENCES users(id) ON DELETE SET NULL
);

-- The idempotency guard. Partial so that failures can be retried freely while a
-- success is recorded once - the same shape as the partial unique index on
-- issuances.program_locator.
CREATE UNIQUE INDEX idx_promo_enrollments_unique_success
    ON promo_enrollments (url_key, document_id) WHERE status = 'success';

CREATE INDEX idx_promo_enrollments_url_key ON promo_enrollments (url_key);
CREATE INDEX idx_promo_enrollments_created_at ON promo_enrollments (created_at DESC);

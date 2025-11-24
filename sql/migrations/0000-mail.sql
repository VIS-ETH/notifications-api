-- +migrate Up
CREATE TYPE mail_status AS ENUM ('submitted', 'processing', 'failed', 'sent');

-- Used for rate limiting and queuing
-- Should be regularly emptied
--
-- Table is _intentionally_ simple. Do not call it disgusting,
-- but there is simply no need to index anything other than the from (with the use case of rate-limiting)
CREATE TABLE mails (
    id SERIAL PRIMARY KEY,

    -- Not part of mail itself, but keeps track of which
    -- rate-limited entity sent this mail.
    rate_limit_user TEXT NOT NULL,

    from_address TEXT,
    reply_to TEXT[],
    to_addresses TEXT[],
    cc_addresses TEXT[],
    bcc_addresses TEXT[],

    extra_headers JSONB,
    subject TEXT,
    body TEXT,

    message_id TEXT NOT NULL,

    status mail_status NOT NULL DEFAULT 'submitted',

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at TIMESTAMPTZ
);

CREATE INDEX mail_rate_limit_user_index ON mails (rate_limit_user);
CREATE INDEX mail_creation_date_index ON mails (created_at);

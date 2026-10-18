-- Migrate up and down to add a new date column for the mails date header

-- +migrate Up
ALTER TABLE mails
    ADD COLUMN date_header TIMESTAMPTZ NOT NULL DEFAULT NOW();

-- +migrate Down
ALTER TABLE mails
    DROP COLUMN date_header;

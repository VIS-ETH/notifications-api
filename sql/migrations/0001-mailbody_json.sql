-- +migrate Up

ALTER TABLE mails
    ALTER COLUMN body TYPE JSONB
    USING CASE WHEN body IS NULL THEN NULL
        ELSE jsonb_build_object(
            'v', 1,
            'root', jsonb_build_object('ct', 'text/plain; charset=utf-8', 'text', body)
        ) END;

-- +migrate Down

ALTER TABLE mails
    ALTER COLUMN body TYPE TEXT USING body->'root'->>'text';

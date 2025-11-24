-- name: PopMailForProcessing :many
WITH next_mail AS (
  SELECT * FROM mails AS m
  WHERE
      m.status = 'submitted'
    OR
      (
          (m.status = 'failed' OR m.status = 'processing')
        AND
          m.processed_at - NOW() > $1::interval
      )
  LIMIT 1
)
UPDATE mails m
SET status = 'processing', processed_at = NOW()
FROM next_mail nm
WHERE nm.id = m.id
RETURNING *;

-- name: SetMailStatus :exec
UPDATE mails
SET status = $1
WHERE id = $2;

-- name: CreateMail :exec
INSERT INTO mails (
  rate_limit_user,
  from_address,
  reply_to,
  to_addresses,
  cc_addresses,
  bcc_addresses,
  extra_headers,
  subject,
  body,
  message_id
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
);

-- name: DeleteOldMails :exec
DELETE FROM mails
WHERE
    status = 'sent'
  AND
    processed_at - NOW() > $1::interval;

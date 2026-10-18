package database_test

import (
	"bytes"
	"net/mail"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal/database"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/pkg/mailer"
)

func TestMailIdentical(t *testing.T) {
	tests := []struct {
		name string
		mail mailer.Mail
	}{
		{
			name: "plain text",
			mail: mailer.Mail{
				ContentType: "text/plain; charset=utf-8",
				From: mail.Address{
					Name:    "Test Sender",
					Address: "test-1@local.local",
				},
				Subject: "Test Subject",
				Body: mailer.Text{
					Content: "Test Body\ntesthere\rit works\n ",
				},
			},
		},
		{
			name: "plain text, cc, bcc, ...",
			mail: mailer.Mail{
				ContentType: "text/plain; charset=utf-8",
				Cc: []mail.Address{
					{Name: "Test CC", Address: "test-2@cc.local"},
					{Name: "Yet another CC", Address: "anything@local.local"},
					{Name: "Test CC", Address: "creative@local.local"},
				},
				Bcc: []mail.Address{
					// random mails
					{Name: "Test BCC", Address: "bcc@local.cc.local"},
					{Name: "Black Carbän", Address: "black@carbon.copied.local"},
				},
				ReplyTo: []mail.Address{
					{Name: "Admin", Address: "admin@local.local"},
				},
				To: []mail.Address{
					{Name: "User", Address: "user@local.local"},
				},
				Date:      time.Now(),
				MessageID: "test123456789_plain_everything+cc+bcc+_etc_@message-id.local",
				Headers: []mailer.Header{
					{Key: "X-Test-Header", Value: "Test Value"},
					{Key: "X-Another-Header", Value: "Another Value"},
				},

				From: mail.Address{
					Name:    "Test Sender",
					Address: "test-1@local.local",
				},
				Subject: "Test Subject",
				Body: mailer.Text{
					Content: "Test Body\ntesthere\rit works\n ",
				},
			},
		},

		{
			name: "simple mail with attachment",
			mail: mailer.Mail{
				ContentType: "multipart/mixed; boundary=boundary",
				From:        mail.Address{Name: "Test Sender", Address: "test@sender.local"},
				To: []mail.Address{
					{Name: "Test Recipient", Address: "test@recipient.local"},
				},
				Subject: "Test Subject with Attachment",
				Body: mailer.Multipart{
					Children: []mailer.Child{
						{
							Headers:     []mailer.Header{},
							ContentType: "text/plain; charset=utf-8",
							Part: mailer.Text{
								Content: "This is the text part of the email.",
							},
						},
						{
							ContentType: "application/octet-stream",
							Headers: []mailer.Header{
								{Key: "Content-Disposition", Value: `attachment; filename="test.txt"`},
							},
							Part: mailer.Binary{
								Content: bytes.Map(func(r rune) rune { return (r + 1337) % 0xff },
									[]byte("Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua.")),
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dbEntity, err := database.MailToDBEntity(&tt.mail)
			require.NoError(t, err, "converting to DB format")

			t.Logf("DB Entity: %+v", dbEntity)
			t.Logf("DB Entity Body: %s", string(dbEntity.Body))

			convertedMail, err := database.DBEntityToMail(*dbEntity)
			require.NoError(t, err, "converting back from DB format")

			assert.Equal(t, tt.mail, *convertedMail)
		})
	}
}

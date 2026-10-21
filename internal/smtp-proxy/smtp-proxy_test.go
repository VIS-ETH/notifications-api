package smtpproxy_test

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	authmock "gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal/auth/mock"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal/testutil"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/pkg/mailer"
)

func TestPassthroughAuth(t *testing.T) {
	te := testutil.NewTestEnvironment(t)
	defer te.Cancel()

	authMockSrv := te.StartAuthMockServer()

	mockGrpcServer := te.GetMockMailerGRPCServer(authMockSrv)
	mockMailer := mockGrpcServer.Mailer
	querier := mockGrpcServer.Querier
	grpcServer := mockGrpcServer.GrpcServer

	client := te.StartGRPCMailServer(grpcServer)

	smtpSrv := te.GetSMTPProxy(authMockSrv, nil, client)
	smtpListening := te.StartSMTPProxyServer(te.Ctx, smtpSrv)

	calls := []*mock.Call{
		mockMailer.EXPECT().MessageIDSuffix().Return("testing-mail-suffix").Call,
		mockMailer.EXPECT().GetSender(mock.Anything).Return(mail.Address{Name: "Test From Name", Address: "test-from@local"}).Call,
		mockMailer.EXPECT().TransmitMail(mock.Anything, mock.Anything).Run(func(_ context.Context, m *mailer.Mail) {
			assert.Equal(t, "test-from@local", m.From.Address)
			assert.Equal(t, "Test From Name", m.From.Name)
			assert.Len(t, m.To, 1)
			assert.Equal(t, "", m.To[0].Name)
			assert.Equal(t, "test2@local.local", m.To[0].Address)
			assert.Equal(t, "Test", m.Subject)
			assert.True(t, strings.HasSuffix(m.MessageID, "testing-mail-suffix"))
			assert.Equal(t, mailer.Binary{Content: []byte("This is a test email.\r\n")}, m.Body)
		}).Return(nil).Call,
		querier.EXPECT().CreateMail(mock.Anything, mock.Anything).Return(nil).Call,
	}
	for _, call := range calls {
		call.Once()
	}
	mock.InOrder(calls...)

	te.TestSMTPClient(smtpListening, func(addr string) error {
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			return fmt.Errorf("failed to parse SMTP server address %q: %w", addr, err)
		}
		smtpAuth := smtp.PlainAuth("", authmock.AuthMockUserUsername, authmock.AuthMockUserPassword, host)
		msg := []byte(
			"From: test@local.local\r\n" +
				"To: test2@local.local\r\n" +
				"Subject: Test\r\n" +
				"\r\n" +
				"This is a test email.\r\n",
		)

		err = smtp.SendMail(addr, smtpAuth, "test@local.local", []string{"test2@local.local"}, msg)
		if err != nil {
			return fmt.Errorf("failed to write email data: %w", err)
		}

		t.Logf("SMTP client finished sending email, err: %v", err)

		mockMailer.AssertExpectations(t)
		querier.AssertExpectations(t)
		te.Cancel()

		return err
	})
}

func TestTLS(t *testing.T) {
	te := testutil.NewTestEnvironment(t)
	defer te.Cancel()

	mockGrpcServer := te.GetMockMailerGRPCServer(nil)
	mockMailer := mockGrpcServer.Mailer
	querier := mockGrpcServer.Querier
	grpcServer := mockGrpcServer.GrpcServer

	client := te.StartGRPCMailServer(grpcServer)

	tlsConfig, roots := te.LoadTestingTLSConfig()
	srv := te.GetSMTPProxy(nil, tlsConfig, client)
	smtpListening := te.StartSMTPProxyServer(te.Ctx, srv)

	calls := []*mock.Call{
		mockMailer.EXPECT().MessageIDSuffix().Return("testing-mail-suffix").Call,
		mockMailer.EXPECT().GetSender(mock.Anything).Return(mail.Address{Name: "Test From Name", Address: "test-from@local"}).Call,
		mockMailer.EXPECT().TransmitMail(mock.Anything, mock.Anything).Run(func(_ context.Context, m *mailer.Mail) {
			assert.Equal(t, "test-from@local", m.From.Address)
			assert.Equal(t, "Test From Name", m.From.Name)
			assert.Len(t, m.To, 1)
			assert.Equal(t, "", m.To[0].Name)
			assert.Equal(t, "test2@local.local", m.To[0].Address)
			assert.Equal(t, "Test", m.Subject)
			assert.True(t, strings.HasSuffix(m.MessageID, "testing-mail-suffix"))
		}).Return(nil).Call,
		querier.EXPECT().CreateMail(mock.Anything, mock.Anything).Return(nil).Call,
	}
	for _, call := range calls {
		call.Once()
	}
	mock.InOrder(calls...)

	te.TestSMTPClient(smtpListening, func(addr string) error {
		conn, err := tls.Dial("tcp", addr, &tls.Config{
			RootCAs:    roots,
			ServerName: "localhost",
		})
		if err != nil {
			return fmt.Errorf("failed to connect with TLS: %w", err)
		}
		smtpClient, err := smtp.NewClient(conn, "test")
		if err != nil {
			return fmt.Errorf("failed to create SMTP client: %v", err)
		}
		t.Logf("Sending HELLO")

		if err := smtpClient.Hello("localhost"); err != nil {
			return fmt.Errorf("failed to send HELO: %v", err)
		}

		if err := smtpClient.Mail("test@local.local"); err != nil {
			return fmt.Errorf("failed to send MAIL FROM: %v", err)
		}
		if err := smtpClient.Rcpt("test2@local.local"); err != nil {
			return fmt.Errorf("failed to send RCPT TO: %v", err)
		}
		wc, err := smtpClient.Data()
		if err != nil {
			return fmt.Errorf("failed to send DATA: %v", err)
		}
		_, err = wc.Write([]byte(
			"From: test@local.local\r\n" +
				"To: test2@local.local\r\n" +
				"Subject: Test\r\n" +
				"\r\n" +
				"This is a test email.\r\n",
		))
		if err != nil {
			return fmt.Errorf("failed to write email data: %w", err)
		}
		err = errors.Join(wc.Close(), smtpClient.Quit())

		t.Logf("SMTP client finished sending email, err: %v", err)
		mockMailer.AssertExpectations(t)
		querier.AssertExpectations(t)
		return err
	})
}

func TestSMTPProxyRejectsNonReencodable(t *testing.T) {
	te := testutil.NewTestEnvironment(t)
	defer te.Cancel()

	authMockSrv := te.StartAuthMockServer()

	mockGrpcServer := te.GetMockMailerGRPCServer(authMockSrv)
	grpcServer := mockGrpcServer.GrpcServer

	client := te.StartGRPCMailServer(grpcServer)

	smtpSrv := te.GetSMTPProxy(authMockSrv, nil, client)
	smtpListening := te.StartSMTPProxyServer(te.Ctx, smtpSrv)

	te.TestSMTPClient(smtpListening, func(addr string) error {
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			return fmt.Errorf("failed to parse SMTP server address %q: %w", addr, err)
		}
		smtpAuth := smtp.PlainAuth("", authmock.AuthMockUserUsername, authmock.AuthMockUserPassword, host)
		msg := []byte(
			"From: test@local.local\r\n" +
				"To: test2@local.local\r\n" +
				// The following few headers should cause issues!
				// Encoding must be downgradable to 7bit by design... because yeah, f mail...
				"Content-Type: message/rfc822\r\n" +
				"Content-Transfer-Encoding: 8bit\r\n" +
				"Mime-Version: 1.0\r\n" +
				"Subject: Test\r\n" +
				"\r\n" +
				"This is a täst email.\r\n",
		)

		err = smtp.SendMail(addr, smtpAuth, "test@local.local", []string{"test2@local.local"}, msg)
		assert.Error(t, err, "This should have failed! SMTP Proxy MUST be able to re-encode everything, so...")

		te.Cancel()

		return nil
	})
}

func TestMailDecode(t *testing.T) {
	mailConversionTests := []struct {
		name        string
		partialMail mailer.Mail
		smtpMessage string
		smtpFrom    string
		smtpTo      []string
	}{
		{
			name:     "plain SMTP text message",
			smtpFrom: "test@local.local",
			smtpTo:   []string{"test2@local.local"},
			smtpMessage: `
From: test@local.local
To: test2@local.local
Subject: Test subject here
Content-Type: text/plain; charset="utf-8"
Content-Transfer-Encoding: quoted-printable
MIME-Version: 1.0

Hallo Beispiel Mail

Unsere garantierten =C3=96ffnungszeiten sind unter der Woche von 12:15 bis =
13:00. M=C3=B6glicherweise findest du aber auch jemanden ausserhalb der =C3=
=96ffnungszeiten im oVIS.
Wir w=C3=BCnschen dir noch einen sch=C3=B6nen Tag.

Liebe Gr=C3=BCsse
Deine VIS IT
`,
			partialMail: mailer.Mail{
				ContentType: "text/plain; charset=\"utf-8\"",
				From:        mail.Address{Name: "Test From Name", Address: "test@local.local"},
				To:          []mail.Address{{Name: "", Address: "test2@local.local"}},
				Subject:     "Test subject here",
				Body: mailer.Text{Content: `Hallo Beispiel Mail

Unsere garantierten Öffnungszeiten sind unter der Woche von 12:15 bis 13:00. Möglicherweise findest du aber auch jemanden ausserhalb der Öffnungszeiten im oVIS.
Wir wünschen dir noch einen schönen Tag.

Liebe Grüsse
Deine VIS IT
`},
			},
		},
		{
			name:     "multipart mime example",
			smtpFrom: "alice@example.com",
			smtpTo:   []string{"bob@example.net"},
			smtpMessage: `
From: Alice Smith <alice@example.com>
To: Bob Jones <bob@example.net>
Subject: Quarterly Report & Summary
Date: Wed, 11 Mar 2026 15:30:00 -0500
MIME-Version: 1.0
Content-Type: multipart/mixed; boundary="----=_NextPart_000_001A_01D8ABCD.12345678"

This is a multi-part message in MIME format.

------=_NextPart_000_001A_01D8ABCD.12345678
Content-Type: multipart/alternative; boundary="----=_NextPart_001_001B_01D8ABCD.12345678"

------=_NextPart_001_001B_01D8ABCD.12345678
Content-Type: text/plain; charset="utf-8"
Content-Transfer-Encoding: 7bit

Hi Bob,

Please find the quarterly report attached and view the summary below.

Best,
Alice

------=_NextPart_001_001B_01D8ABCD.12345678
Content-Type: text/html; charset="utf-8"
Content-Transfer-Encoding: 7bit

<html>
  <body>
    <p>Hi Bob,</p>
    <p>Please find the quarterly report attached and view the summary below.</p>
    <p>Best,<br><strong>Alice</strong></p>
  </body>
</html>

------=_NextPart_001_001B_01D8ABCD.12345678--

------=_NextPart_000_001A_01D8ABCD.12345678
Content-Type: application/pdf; name="Quarterly_Report_2026.pdf"
Content-Transfer-Encoding: base64
Content-Disposition: attachment; filename="Quarterly_Report_2026.pdf"

JVBERi0xLjQKJcOkw7zDtsOfCjIgMCBvYmoKPDwvTGVuZ3RoIDMgMCBSL0ZpbHRlci9GbGF0ZURl
Y29kZT4+CnN0cmVhbQp4nDcIsJQxFADAkf29z11zJ0gq
------=_NextPart_000_001A_01D8ABCD.12345678--
`,
			partialMail: mailer.Mail{
				ContentType: "multipart/mixed; boundary=\"----=_NextPart_000_001A_01D8ABCD.12345678\"",
				From:        mail.Address{Name: "Test From Name", Address: "test@local.local"},
				To:          []mail.Address{{Name: "Bob Jones", Address: "bob@example.net"}},
				Subject:     "Quarterly Report & Summary",
				Body: mailer.Multipart{
					Children: []mailer.Child{
						{
							ContentType: "multipart/alternative; boundary=\"----=_NextPart_001_001B_01D8ABCD.12345678\"",
							Part: mailer.Multipart{
								Children: []mailer.Child{
									{
										ContentType: "text/plain; charset=\"utf-8\"",
										Part: mailer.Text{
											Content: `Hi Bob,

Please find the quarterly report attached and view the summary below.

Best,
Alice
`,
										},
									},
									{
										ContentType: "text/html; charset=\"utf-8\"",
										Part: mailer.Text{
											Content: `<html>
  <body>
    <p>Hi Bob,</p>
    <p>Please find the quarterly report attached and view the summary below.</p>
    <p>Best,<br><strong>Alice</strong></p>
  </body>
</html>
`,
										},
									},
								},
							},
						},
						{
							ContentType: "application/pdf; name=\"Quarterly_Report_2026.pdf\"",
							Headers: []mailer.Header{
								{Key: "Content-Disposition", Value: "attachment; filename=\"Quarterly_Report_2026.pdf\""},
							},
							Part: mailer.Binary{
								Content: []byte{
									0x25, 0x50, 0x44, 0x46, 0x2d, 0x31, 0x2e, 0x34, 0x0a, 0x25, 0xc3, 0xa4,
									0xc3, 0xbc, 0xc3, 0xb6, 0xc3, 0x9f, 0x0a, 0x32, 0x20, 0x30, 0x20, 0x6f,
									0x62, 0x6a, 0x0a, 0x3c, 0x3c, 0x2f, 0x4c, 0x65, 0x6e, 0x67, 0x74, 0x68,
									0x20, 0x33, 0x20, 0x30, 0x20, 0x52, 0x2f, 0x46, 0x69, 0x6c, 0x74, 0x65,
									0x72, 0x2f, 0x46, 0x6c, 0x61, 0x74, 0x65, 0x44, 0x65, 0x63, 0x6f, 0x64,
									0x65, 0x3e, 0x3e, 0x0a, 0x73, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x0a, 0x78,
									0x9c, 0x37, 0x08, 0xb0, 0x94, 0x31, 0x14, 0x00, 0xc0, 0x91, 0xfd, 0xbd,
									0xcf, 0x5d, 0x73, 0x27, 0x48, 0x2a,
								},
							},
						},
					},
				},
			},
		},
	}

	for _, test := range mailConversionTests {
		t.Run(test.name, func(t *testing.T) {
			te := testutil.NewTestEnvironment(t)
			defer te.Cancel()

			mockGrpcServer := te.GetMockMailerGRPCServer(nil)
			mockMailer := mockGrpcServer.Mailer
			querier := mockGrpcServer.Querier
			grpcServer := mockGrpcServer.GrpcServer

			client := te.StartGRPCMailServer(grpcServer)

			smtpSrv := te.GetSMTPProxy(nil, nil, client)
			smtpListening := te.StartSMTPProxyServer(te.Ctx, smtpSrv)

			calls := []*mock.Call{
				mockMailer.EXPECT().MessageIDSuffix().Return("testing-mail-suffix").Call,
				mockMailer.EXPECT().GetSender(mock.Anything).Return(mail.Address{Name: "Test From Name", Address: "test@local.local"}).Call,
				mockMailer.EXPECT().TransmitMail(mock.Anything, mock.Anything).Run(func(_ context.Context, m *mailer.Mail) {
					assert.NotZero(t, m.Date, "mail time should be set now")
					assert.True(t, strings.HasSuffix(m.MessageID, "@testing-mail-suffix"))
					m.MessageID = ""

					m.Date = time.Time{}
					assert.Equal(t, test.partialMail, *m)
				}).Return(nil).Call,
				querier.EXPECT().CreateMail(mock.Anything, mock.Anything).Return(nil).Call,
			}
			for _, call := range calls {
				call.Once()
			}
			mock.InOrder(calls...)

			te.TestSMTPClient(smtpListening, func(addr string) error {
				msg := []byte(strings.ReplaceAll(strings.TrimSpace(test.smtpMessage), "\n", "\r\n"))

				err := smtp.SendMail(addr, nil, test.smtpFrom, test.smtpTo, msg)
				if err != nil {
					return fmt.Errorf("failed to write email data: %w", err)
				}

				t.Logf("SMTP client finished sending email")

				mockMailer.AssertExpectations(t)
				querier.AssertExpectations(t)

				return err
			})
		})
	}
}

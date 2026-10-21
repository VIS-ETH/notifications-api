package mailer_test

import (
	"context"
	"net/mail"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal/testutil"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/pkg/mailer"
)

func TestE2EMailerAndProxy(t *testing.T) {
	te := testutil.NewTestEnvironment(t)
	defer te.Cancel()

	mockGrpcServer := te.GetMockMailerGRPCServer(nil)
	mockMailer := mockGrpcServer.Mailer
	querier := mockGrpcServer.Querier
	grpcServer := mockGrpcServer.GrpcServer

	client := te.StartGRPCMailServer(grpcServer)

	smtpSrv := te.GetSMTPProxy(nil, nil, client)
	smtpListening := te.StartSMTPProxyServer(te.Ctx, smtpSrv)

	te.TestSMTPClient(smtpListening, func(addr string) error {
		mailToCheck := mailer.Mail{
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
		}
		mailToCheck.MessageID = "WILL_BE_WIPED@not-an-example.com"

		calls := []*mock.Call{
			mockMailer.EXPECT().MessageIDSuffix().Return("testing-mail-suffix").Call,
			mockMailer.EXPECT().GetSender(mock.Anything).Return(mail.Address{Name: "Test From Name", Address: "test@local.local"}).Call,
			mockMailer.EXPECT().TransmitMail(mock.Anything, mock.Anything).Run(func(_ context.Context, m *mailer.Mail) {
				assert.NotZero(t, m.Date, "mail time should be set now")
				assert.True(t, strings.HasSuffix(m.MessageID, "@testing-mail-suffix"))
				m.MessageID = ""
				mailToCheck.MessageID = ""

				m.Date = time.Time{}
				assert.Equal(t, mailToCheck, *m)
			}).Return(nil).Call,
			querier.EXPECT().CreateMail(mock.Anything, mock.Anything).Return(nil).Call,
		}
		for _, call := range calls {
			call.Once()
		}
		mock.InOrder(calls...)

		mailSender, err := mailer.NewSMTPMailSender("default-from@local.local", "Default Sender", "smtp://"+addr, nil, "unique-mailid-suffix")
		if err != nil {
			t.Errorf("Failed to create SMTP mail sender: %v", err)
			return err
		}

		err = mailSender.TransmitMail(te.Ctx, &mailToCheck)
		if err != nil {
			t.Errorf("Failed to transmit mail: %v", err)
			return err
		}

		mockMailer.AssertExpectations(t)
		querier.AssertExpectations(t)

		return nil
	})
}

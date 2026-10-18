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
		return err
	})
}

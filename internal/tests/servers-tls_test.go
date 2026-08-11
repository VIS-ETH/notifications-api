package tests

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"testing"
	"time"

	smtp_emersion "github.com/emersion/go-smtp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	mockery_sql "gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/generated/mockery/generated/sql"
	mockery_mailer "gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/generated/mockery/pkg/mailer"
	pb "gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/generated/pb/sip/notifications"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal/grpcservers"
	smtpproxy "gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal/smtp-proxy"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/pkg/mailer"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

func TestTLS(t *testing.T) {
	querier := mockery_sql.NewMockQuerier(t)
	mockMailer := mockery_mailer.NewMockMailSender(t)

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

	grpcServer := grpc.NewServer()

	mailGrpcServer := grpcservers.NewMailServer(
		false,
		true,
		querier,
		mockMailer,
	)

	healthcheck := health.NewServer()
	grpc_health_v1.RegisterHealthServer(grpcServer, healthcheck)
	pb.RegisterMailServiceServer(grpcServer, mailGrpcServer)

	cert, err := tls.LoadX509KeyPair("../../testdata/test-server.crt", "../../testdata/test-server.key")
	if err != nil {
		t.Fatalf("Failed to load TLS certificate: %v", err)
	}
	roots := x509.NewCertPool()
	serverX509Cert, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatalf("failed to parse server certificate: %v", err)
	}
	roots.AddCert(serverX509Cert)

	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}
	clientConn, err := grpc.NewClient(l.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("failed to connect to gRPC server: %v", err)
	}

	eg, ctx := errgroup.WithContext(context.Background())
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	eg.Go(func() error {
		t.Logf("Serving gRPC at %s", l.Addr().String())
		err := grpcServer.Serve(l)
		if err != nil {
			return fmt.Errorf("failed to serve: %v", err)
		}
		return nil
	})
	eg.Go(func() error {
		<-ctx.Done()
		grpcServer.GracefulStop()
		return nil
	})

	smtpListening := make(chan string)

	var srv *smtp_emersion.Server
	eg.Go(func() error {
		client := pb.NewMailServiceClient(clientConn)
		smtpProxyConfig := smtpproxy.SMTPProxyConfig{
			SMTPAuthMode:     smtpproxy.SMTPAuthModeNone,
			SMTPEnsureSender: false,
			GrpcAuthMode:     smtpproxy.GrpcAuthModeNone,
			LoggingOnly:      false,
			OidcConfig:       &smtpproxy.OIDCConfig{},
		}
		srv, err = smtpproxy.GetSMTPServer(smtpProxyConfig, client)
		srv.EnableREQUIRETLS = true
		srv.TLSConfig = &tls.Config{
			Certificates: []tls.Certificate{cert},
		}
		srv.Addr = "localhost:0"
		if err != nil {
			return fmt.Errorf("failed to get SMTP server: %v", err)
		}

		addr := srv.Addr
		if !srv.LMTP && addr == "" {
			addr = ":smtps"
		}

		l, err := tls.Listen("tcp", addr, srv.TLSConfig)
		if err != nil {
			return fmt.Errorf("failed to listen on %s: %v", addr, err)
		}
		smtpListening <- l.Addr().String()

		err = srv.Serve(l)
		if err != nil {
			return fmt.Errorf("failed to serve SMTP server: %v", err)
		}
		return nil
	})
	eg.Go(func() error {
		<-ctx.Done()
		shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelShutdown()
		return srv.Shutdown(shutdownCtx)
	})

	eg.Go(func() error {
		var addr string
		select {
		case <-ctx.Done():
			return nil
		case addr = <-smtpListening:
			break
		}

		t.Logf("Connecting to SMTP server at %s", addr)
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

		err = smtpClient.Hello("localhost")
		if err != nil {
			return fmt.Errorf("failed to send HELO: %v", err)
		}

		err = smtpClient.Mail("test@local.local")
		if err != nil {
			return fmt.Errorf("failed to send MAIL FROM: %v", err)
		}
		err = smtpClient.Rcpt("test2@local.local")
		if err != nil {
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
		if err == nil {
			cancel()
		}

		t.Logf("SMTP client finished sending email, err: %v", err)

		mockMailer.AssertExpectations(t)

		return err
	})

	err = eg.Wait()
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

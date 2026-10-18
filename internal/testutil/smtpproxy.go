package testutil

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http/httptest"
	"time"

	"github.com/emersion/go-smtp"
	pb "gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/generated/pb/sip/notifications"
	authmock "gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal/auth/mock"
	smtpproxy "gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal/smtp-proxy"
)

func (te *TestEnvironment) GetSMTPProxy(authMockSrv *httptest.Server, tlsCfg *tls.Config, client pb.MailServiceClient) *smtp.Server {
	authEnabled := authMockSrv != nil

	smtpProxyConfig := smtpproxy.SMTPProxyConfig{
		SMTPAuthMode:     smtpproxy.SMTPAuthModeNone,
		GrpcAuthMode:     smtpproxy.GrpcAuthModeNone,
		SMTPEnsureSender: false,
		LoggingOnly:      false,
	}
	if authEnabled {
		smtpProxyConfig.SMTPAuthMode = smtpproxy.SMTPAuthModePlain
		smtpProxyConfig.GrpcAuthMode = smtpproxy.GrpcAuthModeSMTPPassthrough
		smtpProxyConfig.OidcConfig = smtpproxy.NewOIDCConfig(authMockSrv.URL+authmock.AuthMockServerTokenPath, authmock.AuthMockClientID, authmock.AuthMockUserPassword)
	}

	srv, err := smtpproxy.GetSMTPServer(smtpProxyConfig, client)
	if err != nil {
		te.T.Errorf("failed to get SMTP server: %v", err)
		te.T.Fail()
		te.Cancel()
	}

	if tlsCfg != nil {
		srv.EnableREQUIRETLS = true
		srv.AllowInsecureAuth = false
		srv.TLSConfig = tlsCfg
	} else {
		srv.EnableREQUIRETLS = false
		srv.AllowInsecureAuth = true
	}

	return srv
}

func (te *TestEnvironment) LoadTestingTLSConfig() (*tls.Config, *x509.CertPool) {
	cert, err := tls.LoadX509KeyPair("../../testdata/test-server.crt", "../../testdata/test-server.key")
	if err != nil {
		te.T.Errorf("Failed to load TLS certificate: %v", err)
		te.T.Fail()
		te.Cancel()
		return nil, nil
	}
	roots := x509.NewCertPool()
	serverX509Cert, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		te.T.Errorf("failed to parse server certificate: %v", err)
		te.T.Fail()
		te.Cancel()
		return nil, nil
	}
	roots.AddCert(serverX509Cert)

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
	}, roots
}

func (te *TestEnvironment) StartSMTPProxyServer(ctx context.Context, srv *smtp.Server) chan string {
	var err error
	smtpListening := make(chan string, 1)

	srv.Addr = "localhost:0"

	te.Eg.Go(func() error {
		addr := srv.Addr
		if !srv.LMTP && addr == "" {
			addr = ":smtps"
		}

		var l net.Listener
		if srv.TLSConfig != nil {
			l, err = tls.Listen("tcp", addr, srv.TLSConfig)
		} else {
			l, err = net.Listen("tcp", addr)
		}
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

	te.Eg.Go(func() error {
		<-ctx.Done()
		shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelShutdown()
		return srv.Shutdown(shutdownCtx)
	})
	return smtpListening
}

func (te *TestEnvironment) TestSMTPClient(smtpListening <-chan string, fn func(addr string) error) {
	te.T.Helper()

	te.Eg.Go(func() error {
		var addr string
		select {
		case <-te.Ctx.Done():
			return nil
		case addr = <-smtpListening:
		}

		defer te.Cancel()
		te.T.Logf("Connecting to SMTP server at %s", addr)

		return fn(addr)
	})

	if err := te.Eg.Wait(); err != nil {
		te.T.Errorf("Test failed: %v", err)
		te.T.Fail()
	}
}

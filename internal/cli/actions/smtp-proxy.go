package actions

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	pb "gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/generated/pb/sip/notifications"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal/cli/config"
	smtpproxy "gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal/smtp-proxy"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

func HandleSMTPProxy(c *config.SMTPProxyConfig) error {
	logrus.Infof("Starting SMTP Proxy with parameters: %v", map[string]any{
		"Logging Only":             c.CommonConfig.LoggingOnly,
		"GRPC Authentication mode": c.GrpcClientAuthMode,
		"GRPC OIDC Client ID":      c.OIDCServiceAccount.OIDCClientID,
		"OIDC Token Endpoint":      c.OIDCServiceAccount.OIDCTokenEndpoint,
		"SMTP Authentication mode": c.SMTPServerAuth,
		"SMTP Server TLS":          c.SMTPServerTLS,
		"SMTP Allow Insecure Auth": c.SMTPServerAllowInsecureAuth,
		"Log Level":                c.CommonConfig.LogLevel,
		"Export OTEL Traces":       c.CommonConfig.Observability.ExportOtelTraces,
		"Export OTEL Metrics":      c.CommonConfig.Observability.ExportOtelMetrics,
		"Exporter Address":         c.CommonConfig.Observability.PrometheusExporterAddr,
	})

	parsedSMTPAuthMode, err := parseSMTPAuthMode(c.SMTPServerAuth)
	if err != nil {
		return fmt.Errorf("Failed to parse SMTP auth mode: %v", err)
	}
	parsedGrpcAuthMode, err := parseGrpcAuthMode(c.GrpcClientAuthMode)
	if err != nil {
		return fmt.Errorf("Failed to parse gRPC auth mode: %v", err)
	}

	if parsedGrpcAuthMode == smtpproxy.GrpcAuthModeOIDCInject && c.OIDCServiceAccount.OIDCClientID == "" || c.OIDCServiceAccount.OIDCClientSecret == "" {
		return fmt.Errorf("OIDC client ID and secret must be provided for OIDC inject mode")
	}
	if parsedGrpcAuthMode != smtpproxy.GrpcAuthModeNone && c.OIDCServiceAccount.OIDCTokenEndpoint == "" {
		return fmt.Errorf("OIDC token endpoint must be provided for gRPC client auth")
	}
	if parsedGrpcAuthMode == smtpproxy.GrpcAuthModeSMTPPassthrough && parsedSMTPAuthMode == smtpproxy.SMTPAuthModeNone {
		return fmt.Errorf("SMTP auth mode must be set to 'plain' when gRPC auth mode is 'passthrough'")
	}
	if parsedSMTPAuthMode != smtpproxy.SMTPAuthModeNone && !c.SMTPServerTLS && !c.SMTPServerAllowInsecureAuth {
		return fmt.Errorf("SMTP server TLS must be enabled when SMTP auth mode is not 'none'")
	}

	oidcConfig := smtpproxy.NewOIDCConfig(c.OIDCServiceAccount.OIDCTokenEndpoint, c.OIDCServiceAccount.OIDCClientID, c.OIDCServiceAccount.OIDCClientSecret)

	var creds credentials.TransportCredentials
	if c.GrpcServerInsecure {
		creds = insecure.NewCredentials()
	} else {
		pool, _ := x509.SystemCertPool()
		creds = credentials.NewClientTLSFromCert(pool, "")
	}

	clientConn, err := grpc.NewClient(c.GrpcServerAddress,
		grpc.WithTransportCredentials(creds),
	)
	if err != nil {
		return fmt.Errorf("failed to connect to gRPC server: %v", err)
	}
	client := pb.NewMailServiceClient(clientConn)

	smtpProxyConfig := smtpproxy.SMTPProxyConfig{
		SMTPAuthMode:     parsedSMTPAuthMode,
		SMTPEnsureSender: false,
		GrpcAuthMode:     parsedGrpcAuthMode,
		LoggingOnly:      c.CommonConfig.LoggingOnly,
		OidcConfig:       oidcConfig,
	}
	srv, err := smtpproxy.GetSMTPServer(smtpProxyConfig, client)
	srv.Addr = c.SMTPServerAddress
	srv.Domain = "localhost"
	if !c.SMTPServerAllowInsecureAuth {
		srv.EnableREQUIRETLS = c.SMTPServerTLS
	}
	var tlsConfig *tls.Config

	if c.SMTPServerTLS {
		tlsConfig, err = loadTLSConfig(c.SMTPServerTLSCertPath, c.SMTPServerTLSKeyPath)
		if err != nil {
			return fmt.Errorf("Failed to load TLS configuration: %v", err)
		}
	}
	srv.TLSConfig = tlsConfig
	srv.AllowInsecureAuth = c.SMTPServerAllowInsecureAuth

	eg, ctx := errgroup.WithContext(context.Background())

	var httpServer http.Server

	eg.Go(func() error {
		metricsServer := http.NewServeMux()
		metricsServer.Handle("/metrics", promhttp.Handler())
		httpServer = http.Server{
			Addr:    c.CommonConfig.Observability.PrometheusExporterAddr,
			Handler: metricsServer,
		}
		return fmt.Errorf("failed to serve http: %v", httpServer.ListenAndServe())
	})
	eg.Go(func() error {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	})

	eg.Go(func() error {
		err = srv.ListenAndServe()
		return fmt.Errorf("failed to serve SMTP server: %v", err)
	})
	eg.Go(func() error {
		<-ctx.Done()
		return srv.Shutdown(ctx)
	})

	return fmt.Errorf("Item in error group failed: %v", eg.Wait())
}

func parseSMTPAuthMode(input string) (smtpproxy.SMTPAuthMode, error) {
	switch smtpproxy.SMTPAuthMode(input) {
	case "none":
		return smtpproxy.SMTPAuthModeNone, nil
	case "plain":
		return smtpproxy.SMTPAuthModePlain, nil
	default:
		return "", fmt.Errorf("invalid SMTPAuthMode: %s", input)
	}
}

func parseGrpcAuthMode(input string) (smtpproxy.GrpcAuthMode, error) {
	switch smtpproxy.GrpcAuthMode(input) {
	case "passthrough":
		return smtpproxy.GrpcAuthModeSMTPPassthrough, nil
	case "oidc-inject":
		return smtpproxy.GrpcAuthModeOIDCInject, nil
	case "none":
		return smtpproxy.GrpcAuthModeNone, nil
	default:
		return "", fmt.Errorf("invalid GrpcAuthMode: %s", input)
	}
}

func loadTLSConfig(certPath, keyPath string) (*tls.Config, error) {
	if certPath == "" || keyPath == "" {
		return nil, fmt.Errorf("TLS certificate and key paths must be provided")
	}
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load key pair: %w", err)
	}
	return &tls.Config{Certificates: []tls.Certificate{cert}}, nil
}

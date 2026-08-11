package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	pb "gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/generated/pb/sip/notifications"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal/observability"
	smtpproxy "gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal/smtp-proxy"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	// gRPC server config
	// Notifications Server configuration
	loggingOnly := flag.Bool(
		"grpc-logging-only",
		// "local-testing" first mentality...
		strings.ToLower(internal.EnvOrDefault("NOTIFICATIONS_LOGGING_ONLY", "true")) == "true",
		"Only log notifications, without sending",
	)
	grpcClientAuthMode := flag.String(
		"grpc-client-auth",
		"none",
		"Authentication mode to be chosen for grpc client of notifications API",
	)

	// Auth flags
	smtpServerAuth := flag.String(
		"smtp-server-auth",
		"none",
		"SMTP server authentication enabled",
	)
	smtpServerTLS := flag.Bool(
		"smtp-server-tls",
		internal.EnvOrDefault("SIP_AUTH_SMTP_SERVER_TLS", "false") != "false",
		"SMTP server TLS enabled",
	)
	smtpServerAllowInsecureAuth := flag.Bool(
		"smtp-server-allow-insecure-auth",
		internal.EnvOrDefault("SIP_AUTH_SMTP_SERVER_ALLOW_INSECURE_AUTH", "false") == "true",
		"SMTP server allow insecure auth enabled",
	)
	// TLS Configurations
	tlsCertPath := flag.String(
		"tls-cert-path",
		"",
		"Path to the TLS certificate file",
	)
	tlsKeyPath := flag.String(
		"tls-key-path",
		"",
		"Path to the TLS key file",
	)
	oidcClientID := flag.String(
		"oidc-client-id",
		internal.EnvOrDefault("SIP_AUTH_OIDC_CLIENT_ID", "notifications-api"),
		"Client ID used for Notifications API",
	)
	oidcClientSecret := flag.String(
		"oidc-client-secret",
		internal.EnvOrDefault("SIP_AUTH_OIDC_CLIENT_SECRET", "notifications-api"),
		"Client Secret used for Notifications API",
	)
	oidcIssuer := flag.String(
		"oidc-issuer",
		internal.EnvOrDefault("SIP_AUTH_OIDC_ISSUER", "https://keycloak-fake.vis.ethz.ch/realms/VSETH"),
		"Issuer URL for OIDC",
	)

	// General server config
	logLevelFlag := flag.String(
		"log-level",
		internal.EnvOrDefault("LOG_LEVEL", "info"),
		"Setting the log level",
	)
	exportOtelTraces := flag.Bool(
		"export-otel-traces",
		internal.EnvOrDefault("EXPORT_OTEL_TRACES", "false") == "true",
		"Export traces to OTEL endpoints",
	)
	exportOtelMetrics := flag.Bool(
		"export-otel-metrics",
		internal.EnvOrDefault("EXPORT_OTEL_METRICS", "false") == "true",
		"Export metrics to OTEL endpoints",
	)
	prometheusExporterAddr := flag.String(
		"prometheus-exporter-addr",
		internal.EnvOrDefault("PROMETHEUS_EXPORTER_ADDR", ":9002"),
		"address (host:port) to export prometheus metrics on",
	)

	logrus.Infof("Starting SMTP Proxy with parameters: %v", map[string]any{
		"Logging Only":             *loggingOnly,
		"GRPC Authentication mode": *grpcClientAuthMode,
		"GRPC OIDC Client ID":      *oidcClientID,
		"GRPC OIDC Issuer":         *oidcIssuer,
		"SMTP Authentication mode": *smtpServerAuth,
		"SMTP Server TLS":          *smtpServerTLS,
		"SMTP Allow Insecure Auth": *smtpServerAllowInsecureAuth,
		"Log Level":                *logLevelFlag,
		"Export OTEL Traces":       *exportOtelTraces,
		"Export OTEL Metrics":      *exportOtelMetrics,
		"Exporter Address":         *prometheusExporterAddr,
	})

	logLevel, err := logrus.ParseLevel(*logLevelFlag)
	if err != nil {
		logrus.Fatalf("Failed to set log level: %v", err)
	}
	logrus.SetLevel(logLevel)

	res, err := resource.Merge(resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName("smtp-proxy"),
		))
	if err != nil {
		logrus.Fatalf("Failed to create resource. Error: %v", err)
	}
	otelCtx := context.Background()

	if *exportOtelTraces {
		tracerProvider, err := observability.SetupTracer(otelCtx, res)
		if err != nil {
			logrus.Fatalf("Failed to setup observability (tracer): %v", err)
		}
		defer func() {
			if err := tracerProvider.Shutdown(otelCtx); err != nil {
				logrus.Fatalf("Failed to shutdown tracerprovider: %v", err)
			}
		}()
	}

	meterProvider, err := observability.SetupMetrics(otelCtx, *exportOtelMetrics, res)
	if err != nil {
		logrus.Fatalf("Failed to setup observability (tracer): %v", err)
	}
	defer func() {
		if err := meterProvider.Shutdown(otelCtx); err != nil {
			logrus.Fatalf("Failed to shutdown metricsprovider: %v", err)
		}
	}()

	parsedSMTPAuthMode, err := parseSMTPAuthMode(*smtpServerAuth)
	if err != nil {
		logrus.Fatalf("Failed to parse SMTP auth mode: %v", err)
	}
	parsedGrpcAuthMode, err := parseGrpcAuthMode(*grpcClientAuthMode)
	if err != nil {
		logrus.Fatalf("Failed to parse gRPC auth mode: %v", err)
	}

	if parsedGrpcAuthMode == smtpproxy.GrpcAuthModeOIDCInject && *oidcClientID == "" || *oidcClientSecret == "" {
		logrus.Fatalf("OIDC client ID and secret must be provided for OIDC inject mode")
	}
	if parsedGrpcAuthMode != smtpproxy.GrpcAuthModeNone && *oidcIssuer == "" {
		logrus.Fatalf("OIDC issuer must be provided for gRPC client auth")
	}
	if parsedGrpcAuthMode == smtpproxy.GrpcAuthModeSMTPPassthrough && parsedSMTPAuthMode == smtpproxy.SMTPAuthModeNone {
		logrus.Fatalf("SMTP auth mode must be set to 'plain' when gRPC auth mode is 'passthrough'")
	}
	if parsedSMTPAuthMode != smtpproxy.SMTPAuthModeNone && !*smtpServerTLS && !*smtpServerAllowInsecureAuth {
		logrus.Fatalf("SMTP server TLS must be enabled when SMTP auth mode is not 'none'")
	}

	oidcConfig := smtpproxy.NewOIDCConfig(*oidcIssuer, *oidcClientID, *oidcClientSecret)

	clientConn, err := grpc.NewClient("localhost:6781",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		logrus.Fatalf("failed to connect to gRPC server: %v", err)
	}
	client := pb.NewMailServiceClient(clientConn)

	smtpProxyConfig := smtpproxy.SMTPProxyConfig{
		SMTPAuthMode:     parsedSMTPAuthMode,
		SMTPEnsureSender: false,
		GrpcAuthMode:     parsedGrpcAuthMode,
		LoggingOnly:      *loggingOnly,
		OidcConfig:       oidcConfig,
	}
	srv, err := smtpproxy.GetSMTPServer(smtpProxyConfig, client)
	srv.Addr = ":2225"
	srv.Domain = "localhost"
	if !*smtpServerAllowInsecureAuth {
		srv.EnableREQUIRETLS = *smtpServerTLS
	}
	tlsConfig := &tls.Config{}

	if *smtpServerTLS {
		tlsConfig, err = loadTLSConfig(*tlsCertPath, *tlsKeyPath)
		if err != nil {
			logrus.Fatalf("Failed to load TLS configuration: %v", err)
		}
	}
	srv.TLSConfig = tlsConfig
	srv.AllowInsecureAuth = *smtpServerAllowInsecureAuth

	eg, ctx := errgroup.WithContext(context.Background())

	var httpServer http.Server

	eg.Go(func() error {
		metricsServer := http.NewServeMux()
		metricsServer.Handle("/metrics", promhttp.Handler())
		httpServer = http.Server{
			Addr:    *prometheusExporterAddr,
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

	logrus.Fatalf("Item in error group failed: %v", eg.Wait())
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

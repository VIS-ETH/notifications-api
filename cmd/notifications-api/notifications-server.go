package main

import (
	"context"
	"flag"
	"net"
	"os"
	"strings"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/sirupsen/logrus"
	pb "gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/generated/pb/sip/notifications"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal/auth"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal/server"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/pkg/mailer"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
	"google.golang.org/grpc"
)

func main() {
	logrus.Info("Starting Notifications API")

	// SMTP Server
	smtpEndpoint := flag.String(
		"smtp-url",
		EnvOrDefault("SMTP_MAIL_URL", "localhost:2225"),
		"SMTP URL for mail client",
	)

	// Auth flags
	oidcClientID := flag.String(
		"oidc-client-id",
		EnvOrDefault("SIP_AUTH_OIDC_CLIENT_ID", "notifications-api"),
		"Client ID used for Notifications API",
	)
	oidcIssuer := flag.String(
		"oidc-issuer",
		EnvOrDefault("SIP_AUTH_OIDC_ISSUER", "https://keycloak-fake.vis.ethz.ch/realms/VSETH"),
		"Issuer URL for OIDC",
	)
	oidcJwksURL := flag.String(
		"oidc-client-jwks-url",
		EnvOrDefault("SIP_AUTH_OIDC_JWKS_URL", "https://keycloak-fake.vis.ethz.ch/realms/VSETH/protocol/openid-connect/certs"),
		"Client ID used for Notifications API",
	)

	// gRPC server config
	// Notifications Server configuration
	loggingOnly := flag.Bool(
		"grpc-logging-only",
		// "local-testing" first mentality...
		strings.ToLower(EnvOrDefault("NOTIFICATIONS_LOGGING_ONLY", "true")) == "true",
		"Only log notifications, without sending",
	)
	unauthenticatedGrpc := flag.Bool(
		"grpc-unauthenticated",
		strings.ToLower(EnvOrDefault("NOTIFICATIONS_UNAUTHENTICATED", "false")) == "true",
		"Skip authentication checks on incoming gRPC requests",
	)
	addrFlag := flag.String(
		"grpc-addr",
		EnvOrDefault("NOTIFICATIONS_BACKEND_GRPC_PORT", ":6781"),
		"gRPC listen address",
	)

	// General server config
	logLevelFlag := flag.String(
		"log-level",
		EnvOrDefault("LOG_LEVEL", "info"),
		"Setting the log level",
	)
	exportOtel := flag.Bool(
		"export-otel",
		EnvOrDefault("EXPORT_OTEL", "false") == "true",
		"Export metrics & traces to OTEL endpoints")

	flag.Parse()

	logrus.Infof("Starting Notifications API with parameters: %v", map[string]any{
		"Unauthenticated gRPC": *unauthenticatedGrpc,
		"Logging only":         *loggingOnly,
		"gRPC server address":  *addrFlag,
		"SMTP endpoint":        *smtpEndpoint,
	})

	logLevel, err := logrus.ParseLevel(*logLevelFlag)
	if err != nil {
		logrus.Fatalf("Failed to set log level: %v", err)
	}
	logrus.SetLevel(logLevel)

	k, err := keyfunc.NewDefaultCtx(context.Background(), []string{*oidcJwksURL})
	if err != nil {
		logrus.Fatalf("Failed to create a keyfunc.Keyfunc from the server's URL. Error: %v", err)
	}

	var serverOptions []grpc.ServerOption

	if *exportOtel {
		res, err := resource.Merge(resource.Default(),
			resource.NewWithAttributes(semconv.SchemaURL,
				semconv.ServiceName("notifications-server"),
			))
		if err != nil {
			logrus.Fatalf("Failed to create resource. Error: %v", err)
		}

		otelCtx := context.Background()
		metricsExp, err := otlpmetricgrpc.New(otelCtx)
		if err != nil {
			logrus.Fatalf("Failed to startup otlp metrics grpc exporter. Error: %v", err)
		}
		meterProvider := metric.NewMeterProvider(
			metric.WithReader(metric.NewPeriodicReader(metricsExp)),
			metric.WithResource(res),
		)
		defer func() {
			if err := meterProvider.Shutdown(otelCtx); err != nil {
				logrus.Fatalf("Failed to shutdown metricsprovider: %v", err)
			}
		}()

		tracesExp, err := otlptracegrpc.New(otelCtx)
		if err != nil {
			logrus.Fatalf("Failed to startup otlp traces grpc exporter. Error: %v", err)
		}
		tracerProvider := trace.NewTracerProvider(
			trace.WithBatcher(tracesExp),
			trace.WithResource(res),
		)
		defer func() {
			if err := tracerProvider.Shutdown(otelCtx); err != nil {
				logrus.Fatalf("Failed to shutdown tracerprovider: %v", err)
			}
		}()

		otel.SetMeterProvider(meterProvider)
		otel.SetTracerProvider(tracerProvider)

		serverOptions = append(serverOptions, grpc.StatsHandler(otelgrpc.NewServerHandler()))
	}

	serverOptions = append(serverOptions, grpc.UnaryInterceptor(
		auth.GetGrpcAuthInterceptor(oidcIssuer, oidcClientID, unauthenticatedGrpc, k.Keyfunc),
	))
	grpcServer := grpc.NewServer(serverOptions...)

	mailConfig, err := mailer.NewMailSender(
		"serviceaccount@vis.ethz.ch",
		*smtpEndpoint,
	)
	if err != nil {
		logrus.Fatalf("Failed to create mail sender: %v", err)
	}

	notificationsServer := server.NewNotificationsServer(
		loggingOnly,
		unauthenticatedGrpc,
		mailConfig,
	)

	pb.RegisterMailServiceServer(grpcServer, notificationsServer)
	l, err := net.Listen("tcp", *addrFlag)
	if err != nil {
		logrus.Fatalf("Failed to listen: %v", err)
	}
	logrus.Printf("Serving gRPC at %s", l.Addr().String())

	logrus.Fatalf("Failed to serve: %v", grpcServer.Serve(l))
}

func EnvOrDefault(envVar, defaultVal string) string {
	envVal, exists := os.LookupEnv(envVar)
	if exists {
		return envVal
	}
	return defaultVal
}

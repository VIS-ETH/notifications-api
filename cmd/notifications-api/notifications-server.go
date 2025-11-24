package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	pb "gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/generated/pb/sip/notifications"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/generated/sql"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal/auth"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal/database"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal/observability"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal/server"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/pkg/mailer"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
)

func main() {
	logrus.Info("Starting Notifications API")

	// SMTP Server
	smtpEndpoint := flag.String(
		"smtp-url",
		internal.EnvOrDefault("SMTP_MAIL_URL", "smtp://localhost:2225"),
		"SMTP URL for mail client",
	)

	// Auth flags
	oidcClientID := flag.String(
		"oidc-client-id",
		internal.EnvOrDefault("SIP_AUTH_OIDC_CLIENT_ID", "notifications-api"),
		"Client ID used for Notifications API",
	)
	oidcIssuer := flag.String(
		"oidc-issuer",
		internal.EnvOrDefault("SIP_AUTH_OIDC_ISSUER", "https://keycloak-fake.vis.ethz.ch/realms/VSETH"),
		"Issuer URL for OIDC",
	)
	oidcJwksURL := flag.String(
		"oidc-client-jwks-url",
		internal.EnvOrDefault("SIP_AUTH_OIDC_JWKS_URL", "https://keycloak-fake.vis.ethz.ch/realms/VSETH/protocol/openid-connect/certs"),
		"Client ID used for Notifications API",
	)

	// gRPC server config
	// Notifications Server configuration
	loggingOnly := flag.Bool(
		"grpc-logging-only",
		// "local-testing" first mentality...
		strings.ToLower(internal.EnvOrDefault("NOTIFICATIONS_LOGGING_ONLY", "true")) == "true",
		"Only log notifications, without sending",
	)
	unauthenticatedGrpc := flag.Bool(
		"grpc-unauthenticated",
		strings.ToLower(internal.EnvOrDefault("NOTIFICATIONS_UNAUTHENTICATED", "false")) == "true",
		"Skip authentication checks on incoming gRPC requests",
	)
	addrFlag := flag.String(
		"grpc-addr",
		internal.EnvOrDefault("NOTIFICATIONS_BACKEND_GRPC_PORT", ":6781"),
		"gRPC listen address",
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
		"Export traces to OTEL endpoints")
	exportOtelMetrics := flag.Bool(
		"export-otel-metrics",
		internal.EnvOrDefault("EXPORT_OTEL_METRICS", "false") == "true",
		"Export metrics to OTEL endpoints")
	prometheusExporterAddr := flag.String(
		"prometheus-exporter-addr",
		internal.EnvOrDefault("PROMETHEUS_EXPORTER_ADDR", ":9001"),
		"address (host:port) to export prometheus metrics on",
	)

	// DB flags
	dsnFlag := flag.String(
		"database-url",
		internal.EnvOrDefault("POSTGRES_DSN", "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable"),
		"PostgreSQL DSN",
	)
	migrationsDir := flag.String(
		"migrations-dir",
		internal.EnvOrDefault("MIGRATIONS_DIR", "sql/migrations"),
		"Directory containing SQL migrations (sql-migrate)",
	)

	flag.Parse()

	var queries *sql.Queries
	if !*loggingOnly {
		err := database.MigrateDB(dsnFlag, migrationsDir)
		if err != nil {
			log.Fatalf("Failed to perform migrations... Is your database functional? %+v", err)
		}

		pool, err := pgxpool.New(context.Background(), *dsnFlag)
		if err != nil {
			logrus.Fatalf("failed to create db pool: %v", err)
		}
		defer pool.Close()

		queries = sql.New(pool)
	}

	logrus.Infof("Starting Notifications API with parameters: %v", map[string]any{
		"Unauthenticated gRPC": *unauthenticatedGrpc,
		"Logging only":         *loggingOnly,
		"gRPC server address":  *addrFlag,
		"SMTP endpoint":        *smtpEndpoint,
		"Database URL":         *dsnFlag,
		"Migrations dir":       *migrationsDir,
		"Export OTEL Metrics:": *exportOtelMetrics,
		"Export OTEL Traces:":  *exportOtelTraces,
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

	res, err := resource.Merge(resource.Default(),
		resource.NewWithAttributes(semconv.SchemaURL,
			semconv.ServiceName("notifications-server"),
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

	grpcServer := grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.UnaryInterceptor(auth.GetGrpcAuthInterceptor(oidcIssuer, oidcClientID, unauthenticatedGrpc, k.Keyfunc)),
	)

	mailSender, err := mailer.NewMailSender(
		"serviceaccount@vis.ethz.ch",
		*smtpEndpoint,
	)
	if err != nil {
		logrus.Fatalf("Failed to create mail sender: %v", err)
	}

	notificationsServer := server.NewNotificationsServer(
		loggingOnly,
		unauthenticatedGrpc,
		queries,
		mailSender,
	)

	pb.RegisterMailServiceServer(grpcServer, notificationsServer)

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

	if !*loggingOnly {
		eg.Go(func() error {
			return internal.HandleMailQueue(ctx, mailSender, queries)
		})
	}

	eg.Go(func() error {
		l, err := net.Listen("tcp", *addrFlag)
		if err != nil {
			logrus.Fatalf("Failed to listen: %v", err)
		}
		logrus.Printf("Serving gRPC at %s", l.Addr().String())

		return fmt.Errorf("failed to serve: %v", grpcServer.Serve(l))
	})
	eg.Go(func() error {
		<-ctx.Done()
		grpcServer.GracefulStop()
		return nil
	})

	logrus.Fatalf("Item in error group failed: %v", eg.Wait())
}

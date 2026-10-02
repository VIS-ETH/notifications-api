package actions

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	pb "gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/generated/pb/sip/notifications"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/generated/sql"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal/auth"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal/cli/config"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal/database"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal/grpcservers"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/pkg/mailer"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/pkg/slack"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

func HandleNotificationsAPI(ctx context.Context, c *config.APIConfig) error {
	var queries *sql.Queries
	if !c.CommonConfig.LoggingOnly {
		err := database.MigrateDB(&c.DatabaseDSN, &c.DatabaseMigrationsDir)
		if err != nil {
			return fmt.Errorf("failed to perform migrations... Is your database functional? %+v", err)
		}

		pool, err := pgxpool.New(ctx, c.DatabaseDSN)
		if err != nil {
			return fmt.Errorf("failed to create db pool: %v", err)
		}
		defer pool.Close()

		queries = sql.New(pool)
	}

	logrus.Infof("Starting Notifications API")

	var jwtKeyFunc func(*jwt.Token) (any, error)
	if c.GrpcUnauthenticated {
		jwtKeyFunc = func(*jwt.Token) (any, error) {
			return nil, nil
		}
	} else {
		k, err := keyfunc.NewDefaultCtx(ctx, []string{c.OIDCConfig.OIDCJWKSURL})
		if err != nil {
			return fmt.Errorf("failed to create a keyfunc.Keyfunc from the server's URL. Error: %v", err)
		}
		jwtKeyFunc = k.Keyfunc
	}

	grpcServer := grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.UnaryInterceptor(auth.GetGrpcAuthInterceptor(c.OIDCConfig.OIDCIssuerURL, c.OIDCConfig.OIDCClientID, jwtKeyFunc)),
	)

	var auth *mailer.SMTPAuth
	if (c.SMTPTargetConfig.AuthUsername == "") != (c.SMTPTargetConfig.AuthPassword == "") {
		logrus.Fatalf("One of plain auth username or password was set, but not both...")
	}
	if c.SMTPTargetConfig.AuthPassword != "" {
		auth = &mailer.SMTPAuth{
			Username: c.SMTPTargetConfig.AuthUsername,
			Password: c.SMTPTargetConfig.AuthPassword,
		}
	}

	meter := otel.Meter("mailer-meter")
	counter, err := meter.Int64Counter(
		"mail_sender_total_mail_count",
		metric.WithDescription("Total mails sent by mailer"),
		metric.WithUnit("{call}"),
	)
	if err != nil {
		logrus.Fatalf("failed to create OTEL counter: %v", err)
	}
	counter.Add(ctx, 4, metric.WithAttributes(attribute.String("backend", "smtp")))

	mailSender, err := mailer.NewSMTPMailSender(
		c.SMTPTargetConfig.DefaultSenderAddress,
		c.SMTPTargetConfig.DefaultSenderName,
		c.SMTPTargetConfig.Endpoint,
		auth,
		c.SMTPTargetConfig.MessageIDSuffix,
	)
	if err != nil {
		logrus.Fatalf("Failed to create mail sender: %v", err)
	}

	mailServer := grpcservers.NewMailServer(
		c.CommonConfig.LoggingOnly,
		c.GrpcUnauthenticated,
		queries,
		mailSender,
	)

	pb.RegisterMailServiceServer(grpcServer, mailServer)

	slackClient := slack.NewClient()
	slackServer := grpcservers.NewSlackServer(
		c.CommonConfig.LoggingOnly,
		c.GrpcUnauthenticated,
		slackClient,
	)

	healthcheck := health.NewServer()
	grpc_health_v1.RegisterHealthServer(grpcServer, healthcheck)
	pb.RegisterSlackMessagingServiceServer(grpcServer, slackServer)

	eg, ctx := errgroup.WithContext(ctx)

	metricsServer := http.NewServeMux()
	httpServer := http.Server{
		Addr:    c.CommonConfig.Observability.PrometheusExporterAddr,
		Handler: metricsServer,
	}
	eg.Go(func() error {
		metricsServer.Handle("/metrics", promhttp.Handler())
		return fmt.Errorf("failed to serve http: %v", httpServer.ListenAndServe())
	})
	eg.Go(func() error {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	})

	if !c.CommonConfig.LoggingOnly {
		eg.Go(func() error {
			return internal.HandleMailQueue(ctx, mailSender, queries)
		})
		eg.Go(func() error {
			return internal.DeleteOldMails(ctx, queries)
		})
	}

	eg.Go(func() error {
		l, err := net.Listen("tcp", c.GrpcAddr)
		if err != nil {
			return fmt.Errorf("failed to listen: %v", err)
		}
		logrus.Infof("Serving gRPC at %s", l.Addr().String())

		err = grpcServer.Serve(l)
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

	return fmt.Errorf("item in error group failed: %v", eg.Wait())
}

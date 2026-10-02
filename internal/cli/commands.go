package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal/cli/actions"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal/cli/config"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal/observability"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
)

type Middleware func(CobraRunE) CobraRunE

const (
	OtelSendAfterShutdownTimeout = 20 * time.Second
)

type CobraRunE func(*cobra.Command, []string) error

var rootConfig = &config.CommonConfig{}

var rootCmd = &cobra.Command{
	Use:          "notifications-api",
	Short:        "Central API managing notifications and messaging",
	SilenceUsage: true,

	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		parsedLogLevel, err := logrus.ParseLevel(rootConfig.LogLevel)
		if err != nil {
			return fmt.Errorf("failed to parse env-set log level: %v", err)
		}
		logrus.SetLevel(parsedLogLevel)
		logrus.Infof("Log level set to %s", parsedLogLevel.String())

		return nil
	},
}

func Init() {
	config.RegisterCommon(rootCmd.PersistentFlags(), rootConfig)

	rootCmd.AddCommand(getSMTPProxyCommand(rootConfig))
	rootCmd.AddCommand(getGrpcApiCommand(rootConfig))
}

func Execute() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	go func() { <-ctx.Done(); cancel() }()

	err := rootCmd.ExecuteContext(ctx)
	cancel()
	if err != nil {
		os.Exit(1)
	}
}

func getSMTPProxyCommand(rootConfig *config.CommonConfig) *cobra.Command {
	c := &config.SMTPProxyConfig{}
	subcmd := &cobra.Command{
		Use: "smtp-proxy",
		RunE: chain(
			func(cmd *cobra.Command, args []string) error {
				return actions.HandleSMTPProxy(c)
			},
			withStartupLog(rootConfig, c),
			withObservability(rootConfig),
		),
	}
	fs := subcmd.Flags()
	config.RegisterSMTPProxy(fs, c, rootConfig)
	return subcmd
}

func getGrpcApiCommand(rootConfig *config.CommonConfig) *cobra.Command {
	c := &config.APIConfig{}
	subcmd := &cobra.Command{
		Use: "grpc-api",
		RunE: chain(
			func(cmd *cobra.Command, args []string) error {
				return actions.HandleNotificationsAPI(cmd.Context(), c)
			},
			withStartupLog(rootConfig, c),
			withObservability(rootConfig),
		),
	}
	fs := subcmd.Flags()
	config.RegisterAPI(fs, c, rootConfig)
	return subcmd
}

func chain(run CobraRunE, mws ...Middleware) CobraRunE {
	for i := len(mws) - 1; i >= 0; i-- {
		run = mws[i](run)
	}
	return run
}

func withStartupLog(rootConfig *config.CommonConfig, subConfig any) Middleware {
	return func(next CobraRunE) CobraRunE {
		return func(cmd *cobra.Command, args []string) error {
			fields, err := config.ConfigFields(rootConfig.LogStartupOptions, *cmd.Flags(), subConfig)
			if err != nil {
				return err
			}
			if fields != nil {
				logrus.WithFields(fields).
					Infof("starting %s", cmd.Name())
			}
			return next(cmd, args)
		}
	}
}

func withObservability(rootConfig *config.CommonConfig) Middleware {
	return func(next CobraRunE) CobraRunE {
		return func(cmd *cobra.Command, args []string) (err error) {
			ctx := cmd.Context()

			res, err := resource.Merge(resource.Default(),
				resource.NewWithAttributes(
					semconv.SchemaURL,
					semconv.ServiceName(cmd.Use),
				))
			if err != nil {
				return fmt.Errorf("failed to create resource: %w", err)
			}

			if rootConfig.Observability.ExportOtelTraces {
				tracerProvider, terr := observability.SetupTracer(ctx, res)
				if terr != nil {
					return fmt.Errorf("failed to setup observability (tracer): %w", terr)
				}
				defer func() {
					sctx, cancel := context.WithTimeout(context.Background(), OtelSendAfterShutdownTimeout)
					defer cancel()
					if serr := tracerProvider.Shutdown(sctx); serr != nil {
						err = errors.Join(err, fmt.Errorf("failed to shutdown tracerprovider: %w", serr))
					}
				}()
			}

			meterProvider, merr := observability.SetupMetrics(ctx, rootConfig.Observability.ExportOtelMetrics, res)
			if merr != nil {
				return fmt.Errorf("failed to setup observability (metrics): %w", merr)
			}
			defer func() {
				sctx, cancel := context.WithTimeout(context.Background(), OtelSendAfterShutdownTimeout)
				defer cancel()
				if serr := meterProvider.Shutdown(sctx); serr != nil {
					err = errors.Join(err, fmt.Errorf("failed to shutdown metricsprovider: %w", serr))
				}
			}()

			return next(cmd, args)
		}
	}
}

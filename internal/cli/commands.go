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

const (
	OtelSendAfterShutdownTimeout = 20 * time.Second
)

type CobraRunE func(*cobra.Command, []string) error

var rootCmd = &cobra.Command{
	Use:   "notifications-api",
	Short: "Central API managing notifications and messaging",
}

func Init() {
	rootCmd.AddCommand(getSMTPProxyCommand())
	rootCmd.AddCommand(getGrpcApiCommand())
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

func withObservability(cfg config.SubcommandConfig, run CobraRunE) CobraRunE {
	return func(cmd *cobra.Command, args []string) (err error) {
		ctx := cmd.Context()

		res, err := resource.Merge(resource.Default(),
			resource.NewWithAttributes(
				semconv.SchemaURL,
				semconv.ServiceName(cmd.Use),
			))
		if err != nil {
			return fmt.Errorf("Failed to create resource. Error: %v", err)
		}

		if cfg.GetCommonConfig().Observability.ExportOtelTraces {
			tracerProvider, err := observability.SetupTracer(ctx, res)
			if err != nil {
				logrus.Fatalf("Failed to setup observability (tracer): %v", err)
			}
			sctx, cancel := context.WithTimeout(context.Background(), OtelSendAfterShutdownTimeout)
			defer cancel()
			defer func() {
				if err2 := tracerProvider.Shutdown(sctx); err2 != nil {
					err = errors.Join(err, fmt.Errorf("Failed to shutdown tracerprovider: %v", err2))
				}
			}()
		}

		meterProvider, err := observability.SetupMetrics(ctx, cfg.GetCommonConfig().Observability.ExportOtelMetrics, res)
		if err != nil {
			return fmt.Errorf("Failed to setup observability (tracer): %v", err)
		}
		defer func() {
			sctx, cancel := context.WithTimeout(context.Background(), OtelSendAfterShutdownTimeout)
			defer cancel()
			if err2 := meterProvider.Shutdown(sctx); err2 != nil {
				err = errors.Join(err, fmt.Errorf("Failed to shutdown metricsprovider: %v", err2))
			}
		}()

		err = run(cmd, args)
		return
	}
}

func getSMTPProxyCommand() *cobra.Command {
	c := &config.SMTPProxyConfig{}
	subcmd := &cobra.Command{
		Use: "smtp-proxy",
		RunE: withObservability(c, func(cmd *cobra.Command, args []string) error {
			return actions.HandleSMTPProxy(c)
		}),
	}
	fs := subcmd.Flags()
	config.RegisterSMTPProxy(fs, c)
	return subcmd
}

func getGrpcApiCommand() *cobra.Command {
	c := &config.APIConfig{}
	subcmd := &cobra.Command{
		Use: "grpc-api",
		RunE: withObservability(c, func(cmd *cobra.Command, args []string) error {
			return actions.HandleNotificationsAPI(cmd.Context(), c)
		}),
	}
	fs := subcmd.Flags()
	config.RegisterAPI(fs, c)
	return subcmd
}

package config

import (
	"crypto/tls"
	"errors"
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal"
	smtpproxy "gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal/smtp-proxy"
)

type ObservabilityConfig struct {
	ExportOtelTraces       bool
	ExportOtelMetrics      bool
	PrometheusExporterAddr string
}

type SMTPClientConfig struct {
	Endpoint             string
	DefaultSenderName    string
	DefaultSenderAddress string
	AuthUsername         string
	AuthPassword         string `confidential:"true"`
	MessageIDSuffix      string
}

type SMTPServerConfig struct {
	AuthMode      smtpproxy.SMTPAuthMode
	TLSConfig     *tls.Config
	AllowInsecure bool
	OIDCConfig    *OIDCConfig
}

type OIDCConfig struct {
	OIDCClientID     string
	OIDCClientSecret string `confidential:"true"`
	OIDCJWKSURL      string
	OIDCIssuerURL    string
}

type OIDCServiceAccountConfig struct {
	OIDCClientID      string
	OIDCClientSecret  string `confidential:"true"`
	OIDCTokenEndpoint string
}

// General configs
type CommonConfig struct {
	LoggingOnly bool
	LogLevel    logrus.Level

	Observability ObservabilityConfig
	SubcommandConfig
}

type SubcommandConfig interface {
	// Common configuration that is available for all commands
	GetCommonConfig() *CommonConfig
}

func RegisterObservability(fs *pflag.FlagSet, c *ObservabilityConfig) {
	fs.BoolVar(
		&c.ExportOtelTraces,
		"export-otel-traces",
		internal.EnvOrDefault("EXPORT_OTEL_TRACES", "false") == "true",
		"Export traces to OTEL endpoints",
	)
	fs.BoolVar(
		&c.ExportOtelMetrics,
		"export-otel-metrics",
		internal.EnvOrDefault("EXPORT_OTEL_METRICS", "false") == "true",
		"Export metrics to OTEL endpoints",
	)
	fs.StringVar(
		&c.PrometheusExporterAddr,
		"prometheus-exporter-addr",
		internal.EnvOrDefault("PROMETHEUS_EXPORTER_ADDR", ":9001"),
		"address (host:port) to export prometheus metrics on",
	)
}

func RegisterCommon(fs *pflag.FlagSet, c *CommonConfig) {
	envLogLevel := internal.EnvOrDefault("LOG_LEVEL", "info")
	envParsedLogLevel, err := logrus.ParseLevel(envLogLevel)
	if err != nil {
		logrus.Fatalf("Failed to parse env-set log level: %v", err)
	}
	c.LogLevel = envParsedLogLevel

	fs.BoolVar(
		&c.LoggingOnly,
		"logging-only",
		// "local-testing" first mentality...
		strings.ToLower(internal.EnvOrDefault("NOTIFICATIONS_LOGGING_ONLY", "true")) == "true",
		"Only log notifications, without sending",
	)

	setLogLevelAlready := false
	fs.Func(
		"log-level",
		"Setting the log level",
		func(value string) error {
			if setLogLevelAlready {
				return errors.New("log-level flag already set")
			}
			setLogLevelAlready = true
			logLevel, err := logrus.ParseLevel(value)
			if err != nil {
				return fmt.Errorf("Failed to parse log level: %v", err)
			}
			c.LogLevel = logLevel
			return nil
		},
	)

	RegisterObservability(fs, &c.Observability)
}

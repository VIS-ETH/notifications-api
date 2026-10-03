package config

import (
	"crypto/tls"
	"strings"

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
	LoggingOnly       bool
	LogLevel          string
	LogStartupOptions string

	Observability ObservabilityConfig
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
	fs.BoolVar(
		&c.LoggingOnly,
		"logging-only",
		// "local-testing" first mentality...
		strings.ToLower(internal.EnvOrDefault("NOTIFICATIONS_LOGGING_ONLY", "true")) == "true",
		"Only log notifications, without sending",
	)

	fs.StringVar(
		&c.LogLevel,
		"log-level",
		internal.EnvOrDefault("LOG_LEVEL", "info"),
		"Setting the log level",
	)

	fs.StringVar(
		&c.LogStartupOptions,
		"log-startup-options",
		internal.EnvOrDefault("LOG_STARTUP_OPTIONS", "redact-confidential"),
		"Whether to log startup CLI commands. Must be one of [all, redact-confidential, none].",
	)

	RegisterObservability(fs, &c.Observability)
}

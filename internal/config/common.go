package config

import (
	"crypto/tls"
	"flag"
	"strings"

	"github.com/sirupsen/logrus"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal"
	smtpproxy "gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal/smtp-proxy"
)

type ObservabilityConfig struct {
	logLevel               logrus.Level
	exportOtelTraces       bool
	exportOtelMetrics      bool
	prometheusExporterAddr string
}

type SMTPClientConfig struct {
	Endpoint             string
	DefaultSenderName    string
	DefaultSenderAddress string
	AuthUsername         string
	AuthPassword         string
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
	OIDCClientSecret string
	OIDCJWKSURL      string
}

// General configs
type CommonConfig struct {
	LoggingOnly bool
}

func RegisterCommon(fs *flag.FlagSet, args []string) *CommonConfig {
	c := &CommonConfig{}
	fs.BoolVar(
		&c.LoggingOnly,
		"grpc-logging-only",
		// "local-testing" first mentality...
		strings.ToLower(internal.EnvOrDefault("NOTIFICATIONS_LOGGING_ONLY", "true")) == "true",
		"Only log notifications, without sending",
	)

	return c
}

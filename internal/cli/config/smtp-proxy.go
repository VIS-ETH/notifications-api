package config

import (
	"flag"

	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal"
)

type SMTPProxyConfig struct {
	CommonConfig CommonConfig

	GrpcClientAuthMode string
	GrpcServerAddress  string
	GrpcServerInsecure bool

	SMTPServerAuth              string
	SMTPServerTLS               bool
	SMTPServerTLSCertPath       string
	SMTPServerTLSKeyPath        string
	SMTPServerAllowInsecureAuth bool
	SMTPServerAddress           string

	OIDCServiceAccount OIDCServiceAccountConfig
}

func (c *SMTPProxyConfig) GetCommonConfig() *CommonConfig {
	return &c.CommonConfig
}

func (c *SMTPProxyConfig) ObservabilitySetup() bool {
	return true
}

func RegisterSMTPProxy(fs *flag.FlagSet, c *SMTPProxyConfig) {
	fs.StringVar(
		&c.GrpcClientAuthMode,
		"grpc-client-auth",
		internal.EnvOrDefault("GRPC_CLIENT_AUTH_MODE", "none"),
		"Authentication mode to be chosen for grpc client of notifications API",
	)
	fs.StringVar(
		&c.GrpcServerAddress,
		"grpc-server-address",
		internal.EnvOrDefault("GRPC_SERVER_ADDRESS", "localhost:6781"),
		"Address of the gRPC server to connect to",
	)
	fs.BoolVar(
		&c.GrpcServerInsecure,
		"grpc-server-insecure",
		internal.EnvOrDefault("GRPC_SERVER_INSECURE", "true") == "true",
		"Use insecure connection to gRPC server",
	)

	// Auth flags
	fs.StringVar(
		&c.SMTPServerAuth,
		"smtp-server-auth",
		internal.EnvOrDefault("SMTP_CLIENT_AUTH_MODE", "none"),
		"SMTP server authentication enabled",
	)
	fs.BoolVar(
		&c.SMTPServerTLS,
		"smtp-server-tls",
		internal.EnvOrDefault("SMTP_SERVER_TLS", "false") != "false",
		"SMTP server TLS enabled",
	)
	fs.BoolVar(
		&c.SMTPServerAllowInsecureAuth,
		"smtp-server-allow-insecure-auth",
		internal.EnvOrDefault("SMTP_SERVER_ALLOW_INSECURE_AUTH", "false") == "true",
		"SMTP server allow insecure auth enabled",
	)
	fs.StringVar(
		&c.SMTPServerAddress,
		"smtp-server-address",
		internal.EnvOrDefault("SMTP_SERVER_ADDRESS", ":2225"),
		"SMTP server address",
	)
	// TLS Configurations
	fs.StringVar(
		&c.SMTPServerTLSCertPath,
		"tls-cert-path",
		"",
		"Path to the TLS certificate file",
	)
	fs.StringVar(
		&c.SMTPServerTLSKeyPath,
		"tls-key-path",
		"",
		"Path to the TLS key file",
	)
	fs.StringVar(
		&c.OIDCServiceAccount.OIDCClientID,
		"oidc-client-id",
		internal.EnvOrDefault("SIP_AUTH_OIDC_CLIENT_ID", "notifications-api"),
		"Client ID used for Notifications API",
	)
	fs.StringVar(
		&c.OIDCServiceAccount.OIDCClientSecret,
		"oidc-client-secret",
		internal.EnvOrDefault("SIP_AUTH_OIDC_CLIENT_SECRET", "notifications-api"),
		"Client Secret used for Notifications API",
	)
	fs.StringVar(
		&c.OIDCServiceAccount.OIDCTokenEndpoint,
		"oidc-token-endpoint",
		internal.EnvOrDefault("SIP_AUTH_OIDC_TOKEN_ENDPOINT", "https://keycloak-fake.vis.ethz.ch/realms/VSETH/protocol/openid-connect/token"),
		"TokenEndpoint URL for OIDC",
	)
}

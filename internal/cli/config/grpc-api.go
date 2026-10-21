package config

import (
	"strings"

	"github.com/spf13/pflag"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal"
)

type MailerAvailableBackend string

const (
	MailerBackendSMTP       MailerAvailableBackend = "smtp"
	MailerBackendMsGraphAPI MailerAvailableBackend = "ms-graph"
)

type APIConfig struct {
	CommonConfig *CommonConfig

	SMTPTargetConfig       SMTPClientConfig
	MsGraphAPITargetConfig MsGraphAPITargetConfig
	BackendSelection       string
	OIDCConfig             OIDCConfig

	GrpcUnauthenticated bool
	GrpcAddr            string

	DatabaseDSN           string `confidential:"true"`
	DatabaseMigrationsDir string
}

func RegisterAPI(fs *pflag.FlagSet, c *APIConfig, commonConfig *CommonConfig) {
	c.CommonConfig = commonConfig

	// SMTP Target Endpoint Config
	fs.StringVar(
		&c.SMTPTargetConfig.Endpoint,
		"smtp-url",
		internal.EnvOrDefault("SMTP_MAIL_URL", "smtp://localhost:2225"),
		"SMTP URL for mail client",
	)
	fs.StringVar(
		&c.SMTPTargetConfig.DefaultSenderName,
		"smtp-default-sender-name",
		internal.EnvOrDefault("SMTP_DEFAULT_SENDER_NAME", "Mail API"),
		"SMTP default sender name for mails",
	)
	fs.StringVar(
		&c.SMTPTargetConfig.DefaultSenderAddress,
		"smtp-default-sender-address",
		internal.EnvOrDefault("SMTP_DEFAULT_SENDER_ADDRESS", "serviceaccount@ethz.ch"),
		"SMTP default sender address for mails",
	)
	fs.StringVar(
		&c.SMTPTargetConfig.AuthUsername,
		"smtp-auth-login-username",
		internal.EnvOrDefault("SMTP_AUTH_LOGIN_USERNAME", ""),
		"SMTP plain auth username",
	)
	fs.StringVar(
		&c.SMTPTargetConfig.AuthPassword,
		"smtp-auth-login-password",
		internal.EnvOrDefault("SMTP_AUTH_LOGIN_PASSWORD", ""),
		"SMTP plain auth password",
	)
	fs.StringVar(
		&c.SMTPTargetConfig.MessageIDSuffix,
		"message-id-suffix",
		internal.EnvOrDefault("SMTP_MESSAGE_ID_SUFFIX", "mail-api"),
		"Message ID suffix",
	)

	// Microsoft Graph API Config
	fs.StringVar(
		&c.MsGraphAPITargetConfig.SenderName,
		"ms-graph-sender-name",
		internal.EnvOrDefault("MS_GRAPH_SP_SENDER_NAME", "Mail API"),
		"Microsoft Graph API sender name of service principal",
	)
	fs.StringVar(
		&c.MsGraphAPITargetConfig.SenderAddress,
		"ms-graph-sender-address",
		internal.EnvOrDefault("MS_GRAPH_SP_SENDER_ADDRESS", ""),
		"Microsoft Graph API sender address of service principal",
	)
	fs.StringVar(
		&c.MsGraphAPITargetConfig.SPName,
		"ms-graph-sp-name",
		internal.EnvOrDefault("MS_GRAPH_SP_NAME", "mail-api"),
		"Microsoft Graph API service principal display name",
	)
	fs.StringVar(
		&c.MsGraphAPITargetConfig.ClientID,
		"ms-graph-sp-client-id",
		internal.EnvOrDefault("MS_GRAPH_SP_CLIENT_ID", ""),
		"Microsoft Graph API Client ID of service principal",
	)
	fs.StringVar(
		&c.MsGraphAPITargetConfig.ClientSecret,
		"ms-graph-sp-client-secret",
		internal.EnvOrDefault("MS_GRAPH_SP_CLIENT_SECRET", ""),
		"Microsoft Graph API Client Secret of service principal",
	)
	fs.StringVar(
		&c.MsGraphAPITargetConfig.TenantID,
		"ms-graph-tenant-id",
		internal.EnvOrDefault("MS_GRAPH_TENANT_ID", ""),
		"Microsoft Graph API tenant ID",
	)
	fs.StringVar(
		&c.MsGraphAPITargetConfig.MessageIDSuffix,
		"ms-graph-message-id-suffix",
		internal.EnvOrDefault("MS_GRAPH_MESSAGE_ID_SUFFIX", "mail-api"),
		"Message ID suffix",
	)

	// OIDC Config
	fs.StringVar(
		&c.OIDCConfig.OIDCClientID,
		"oidc-client-id",
		internal.EnvOrDefault("SIP_AUTH_OIDC_CLIENT_ID", "notifications-api"),
		"Client ID used for Notifications API",
	)
	fs.StringVar(
		&c.OIDCConfig.OIDCClientSecret,
		"oidc-client-secret",
		internal.EnvOrDefault("SIP_AUTH_OIDC_CLIENT_SECRET", "notifications-api"),
		"Client Secret used for Notifications API",
	)
	fs.StringVar(
		&c.OIDCConfig.OIDCJWKSURL,
		"oidc-client-jwks-url",
		internal.EnvOrDefault("SIP_AUTH_OIDC_JWKS_URL", "https://keycloak-fake.vis.ethz.ch/realms/VSETH/protocol/openid-connect/certs"),
		"OIDC JWKS URL used for Notifications API",
	)
	fs.StringVar(
		&c.OIDCConfig.OIDCIssuerURL,
		"oidc-issuer",
		internal.EnvOrDefault("SIP_AUTH_OIDC_ISSUER", "https://keycloak-fake.vis.ethz.ch/realms/VSETH"),
		"Issuer URL for OIDC",
	)

	// GRPC Server configs
	fs.BoolVar(
		&c.GrpcUnauthenticated,
		"grpc-unauthenticated",
		strings.ToLower(internal.EnvOrDefault("NOTIFICATIONS_UNAUTHENTICATED", "false")) == "true",
		"Skip authentication checks on incoming gRPC requests",
	)
	fs.StringVar(
		&c.GrpcAddr,
		"grpc-addr",
		internal.EnvOrDefault("NOTIFICATIONS_API_GRPC_PORT", ":6781"),
		"gRPC listen address",
	)
	fs.StringVar(
		&c.BackendSelection,
		"grpc-mailer-backend",
		internal.EnvOrDefault("NOTIFICATIONS_API_MAILER_BACKEND", string(MailerBackendSMTP)),
		"Select backend to use for mailer in grpc server",
	)

	// DB flags
	fs.StringVar(
		&c.DatabaseDSN,
		"database-url",
		internal.EnvOrDefault("POSTGRES_DSN", "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable"),
		"PostgreSQL DSN",
	)
	fs.StringVar(
		&c.DatabaseMigrationsDir,
		"migrations-dir",
		internal.EnvOrDefault("MIGRATIONS_DIR", "sql/migrations"),
		"Directory containing SQL migrations (sql-migrate)",
	)
}

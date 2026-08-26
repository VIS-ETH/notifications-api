package config

import (
	"flag"

	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal"
)

type APIConfig struct {
	SMTPTargetConfig SMTPClientConfig
	OIDCConfig       OIDCConfig
}

func RegisterAPI(fs *flag.FlagSet, args []string) *APIConfig {
	c := &APIConfig{}

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

	fs.StringVar(
		&c.OIDCConfig.OIDCClientID,
		"oidc-client-id",
		internal.EnvOrDefault("SIP_AUTH_OIDC_CLIENT_ID", "notifications-api"),
		"Client ID used for Notifications API",
	)
	fs.StringVar(
		&c.OIDCConfig.OIDCClientSecret,
		"oidc-issuer",
		internal.EnvOrDefault("SIP_AUTH_OIDC_ISSUER", "https://keycloak-fake.vis.ethz.ch/realms/VSETH"),
		"Issuer URL for OIDC",
	)
	fs.StringVar(
		&c.OIDCConfig.OIDCJWKSURL,
		"oidc-client-jwks-url",
		internal.EnvOrDefault("SIP_AUTH_OIDC_JWKS_URL", "https://keycloak-fake.vis.ethz.ch/realms/VSETH/protocol/openid-connect/certs"),
		"Client ID used for Notifications API",
	)

	return c
}

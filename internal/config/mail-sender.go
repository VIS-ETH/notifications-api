package config

import (
	"flag"

	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal"
)

type MailSenderConfig struct {
	FromAddress string
	FromName    string
	Body        string
}

func RegisterMailSender(fs *flag.FlagSet, args []string) *MailSenderConfig {
	mailSenderConfig := &MailSenderConfig{}

	fs.StringVar(&mailSenderConfig.FromAddress,
		"from",
		internal.EnvOrDefault("MAIL_FROM", "anonymous@local"),
		"From address of mail",
	)
	fs.StringVar(&mailSenderConfig.FromName,
		"from-name",
		internal.EnvOrDefault("MAIL_FROM_NAME", "Anonymous"),
		"From sender name of mail",
	)
	fs.StringVar(&mailSenderConfig.Body,
		"body",
		internal.EnvOrDefault("MAIL_BODY", ""),
		"",
	)

	return mailSenderConfig
}

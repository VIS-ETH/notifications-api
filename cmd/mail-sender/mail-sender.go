package main

import (
	"context"
	"flag"
	"fmt"
	"net/mail"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/pkg/mailer"
)

func main() {
	// SMTP Server
	smtpEndpoint := flag.String(
		"smtp-url",
		internal.EnvOrDefault("SMTP_MAIL_URL", "localhost:2225"),
		"SMTP URL for mail client",
	)

	// Mail arguments
	from := flag.String(
		"from",
		internal.EnvOrDefault("MAIL_FROM", "anonymous@local"),
		"From address of mail",
	)
	fromName := flag.String(
		"from-name",
		internal.EnvOrDefault("MAIL_FROM_NAME", "Anonymous"),
		"From sender name of mail",
	)

	body := flag.String(
		"body",
		internal.EnvOrDefault("MAIL_BODY", ""),
		"",
	)

	flag.Parse()

	if *body == "" {
		logrus.Fatalf("Body should not be empty")
	}

	mailSender, err := mailer.NewMailSender(
		*from,
		*fromName,
		*smtpEndpoint,
		nil,
		"mail-cli-vis",
	)
	if err != nil {
		logrus.Fatalf("Failed to setup mailsender: %v", err)
	}

	messageUUID, err := uuid.NewV7()
	if err != nil {
		logrus.Fatalf("Failed to generate uuid: %v", err)
	}
	messageID := fmt.Sprintf("%s@%s", messageUUID.String(), "mail-cli-vis")

	err = mailSender.TransmitMail(
		context.Background(),
		&mailer.Mail{
			MessageID: messageID,
			From: &mail.Address{
				Address: *from,
			},
			Body: *body,
		})
	if err != nil {
		logrus.Fatalf("Failed to send mail: %v", err)
	}
}

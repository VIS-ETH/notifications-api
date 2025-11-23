package mailer

import (
	"context"
	"errors"
	"fmt"
	"net/smtp"
	"slices"

	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

type MailSender struct {
	defaultMailSenderAddress string
	smtpEndpoint             string
	logger                   *logrus.Entry
	mailCounter              metric.Int64Counter
}

func NewMailSender(defaultSender string, smtpEndpoint string) (*MailSender, error) {
	mailLogger := logrus.WithFields(logrus.Fields{
		"component": "mail",
	})
	meter := otel.Meter("mailer-meter")
	counter, err := meter.Int64Counter("mail_sender_total_mail_count",
		metric.WithDescription("Total mails sent by mailer"),
		metric.WithUnit("{call}"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTEL counter: %v", err)
	}

	return &MailSender{
		defaultMailSenderAddress: defaultSender,
		smtpEndpoint:             smtpEndpoint,
		logger:                   mailLogger,
		mailCounter:              counter,
	}, nil
}

func (s *MailSender) DefaultSenderAddress() string {
	return s.defaultMailSenderAddress
}

func (s *MailSender) TransmitMail(ctx context.Context, m *Mail) error {
	if m == nil {
		return errors.New("tried to send empty mail")
	}
	if m.MessageID == "" {
		return errors.New("tried to send mail without Message ID")
	}
	logger := s.logger.WithFields(logrus.Fields{
		"message-id": m.MessageID,
	})

	tr := otel.Tracer("mailer/sender")
	_, span := tr.Start(ctx, "smtp.SendMail",
		trace.WithAttributes(
			attribute.String("smtp.server", s.smtpEndpoint),
			attribute.String("smtp.from", m.From.Address),
			attribute.Int("smt.message_size", len(m.Body)),
		),
	)
	defer span.End()
	logger.Trace("Establishing SMTP connection")

	client, err := smtp.Dial(s.smtpEndpoint)
	if err != nil {
		return fmt.Errorf("failed to dial smtp server and retrieve client: %v", err)
	}
	defer func() {
		err = client.Close()
		if err != nil {
			err = fmt.Errorf("failed to close smtp client: %v", err)
		}
	}()

	if err != nil {
		return errors.New("failed to connect mail server")
	}

	logger.Trace("SMTP From")

	if err := client.Mail(m.From.Address); err != nil {
		logger.Errorf("Failed to mail from %s: %v", m.From.Address, err)
		return fmt.Errorf("could not start sending mail with from mail address %s", m.From.Address)
	}

	logger.Trace("Setting SMTP recipients")

	recipients := slices.Concat(m.To, m.Cc, m.Bcc)
	for i, to := range recipients {
		if err := client.Rcpt(to.Address); err != nil {
			logger.Errorf("Failed to add recipient %d (%s): %v", i, to.Address, err)
			return fmt.Errorf("could not add recipient %d: %s", i, to.Address)
		}
	}

	logger.Trace("Transmitting data...")

	wc, err := client.Data()
	if err != nil {
		return fmt.Errorf("could not start sending data: %v", err)
	}
	defer func() {
		err = wc.Close()
		if err != nil {
			err = fmt.Errorf("failed to close data writer: %v", err)
		}
	}()

	_, err = wc.Write([]byte(m.GetMessageContent()))
	if err != nil {
		return fmt.Errorf("failed to write message content: %v", err)
	}

	logger.Trace("Successfully written message")
	s.mailCounter.Add(
		context.Background(), 1,
		metric.WithAttributes(attribute.String("smtp_endpoint", s.smtpEndpoint)))
	span.SetStatus(codes.Ok, "email sent")

	return nil
}

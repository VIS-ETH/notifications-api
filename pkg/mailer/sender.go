package mailer

import (
	"context"
	"errors"
	"fmt"
	"net/smtp"
	"net/url"
	"slices"

	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
	"go.opentelemetry.io/otel/trace"
)

type MailSender struct {
	defaultMailSenderAddress string
	smtpEndpoint             url.URL
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

	smtpEndpointURL, err := url.Parse(smtpEndpoint)
	if err != nil {
		return nil, fmt.Errorf("could not parse SMTP endpoint as URL: %v", err)
	}

	return &MailSender{
		defaultMailSenderAddress: defaultSender,
		smtpEndpoint:             *smtpEndpointURL,
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
		"smtp-from":  m.From.String(),
	})

	tr := otel.Tracer("mailer/sender")
	_, span := tr.Start(ctx, "smtp.SendMail",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("smtp.from", m.From.Address),
			semconv.NetworkProtocolName("smtp"),
			semconv.ServerAddress(s.smtpEndpoint.Hostname()),
			semconv.NetworkProtocolName("smtp"),
			attribute.Int("smtp.message_size", len(m.Body)),
		),
	)
	defer span.End()
	logger.Trace("Establishing SMTP connection")

	client, err := smtp.Dial(s.smtpEndpoint.Host)
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
		metric.WithAttributes(attribute.String("smtp_endpoint", s.smtpEndpoint.Host)))
	span.SetStatus(codes.Ok, "email sent")

	logger.Info("Sent mail successfully")

	return nil
}

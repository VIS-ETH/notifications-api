package mailer

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
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

type SMTPMailSender struct {
	defaultMailSenderAddress string
	defaultMailSenderName    string
	smtpEndpoint             url.URL
	logger                   *logrus.Entry
	mailCounter              metric.Int64Counter
	messageIDSuffix          string
	auth                     *SMTPAuth
}

func NewSMTPMailSender(defaultSenderAddress, defaultSenderName, smtpEndpoint string, smtpAuth *SMTPAuth, messageIDSuffix string) (*SMTPMailSender, error) {
	mailLogger := logrus.WithFields(logrus.Fields{
		"component": "smtp-mail",
	})
	meter := otel.Meter("mailer-meter")
	counter, err := meter.Int64Counter(
		"mail_sender_total_mail_count",
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

	return &SMTPMailSender{
		defaultMailSenderAddress: defaultSenderAddress,
		defaultMailSenderName:    defaultSenderName,
		smtpEndpoint:             *smtpEndpointURL,
		logger:                   mailLogger,
		mailCounter:              counter,
		auth:                     smtpAuth,
		messageIDSuffix:          messageIDSuffix,
	}, nil
}

func (s *SMTPMailSender) GetSender(from mail.Address) mail.Address {
	if from.Name == "" {
		from.Name = s.defaultMailSenderName
	}
	if from.Address == "" {
		from.Address = s.defaultMailSenderAddress
	}
	return from
}

func (s *SMTPMailSender) MessageIDSuffix() string {
	return s.messageIDSuffix
}

func (s *SMTPMailSender) TransmitMail(ctx context.Context, m *Mail) error {
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
	_, span := tr.Start(
		ctx, "smtp.SendMail",
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

	recipients := slices.Concat(m.To, m.Cc, m.Bcc)
	var recipientsAddresses []string
	for _, to := range recipients {
		recipientsAddresses = append(recipientsAddresses, to.Address)
	}

	// avoid typing nil to interface:)
	var smtpAuth smtp.Auth = nil
	if s.auth != nil {
		smtpAuth = s.auth
	}
	err := smtp.SendMail(
		s.smtpEndpoint.Host,
		smtpAuth,
		m.From.Address,
		recipientsAddresses,
		[]byte(m.GetMessageContent()),
	)
	if err != nil {
		return fmt.Errorf("failed to send mail: %v", err)
	}

	logger.Trace("Successfully written message")
	s.mailCounter.Add(
		context.Background(), 1,
		metric.WithAttributes(attribute.String("smtp_endpoint", s.smtpEndpoint.Host)),
	)
	span.SetStatus(codes.Ok, "email sent")

	logger.Info("Sent mail successfully")

	return nil
}

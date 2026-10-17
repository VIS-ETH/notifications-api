package smtpserver

import (
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

type SMTPObservability struct {
	counter *metric.Int64Counter
}

var so *SMTPObservability

func getSMTPObservability() (*SMTPObservability, error) {
	if so != nil {
		return so, nil
	}
	meter := otel.Meter("smtp-server-meter")
	counter, err := meter.Int64Counter(
		"mail_sender_total_mail_count",
		metric.WithDescription("Total mails received by smtp server"),
		metric.WithUnit("{call}"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTEL counter: %v", err)
	}

	so = &SMTPObservability{
		counter: &counter,
	}

	return so, nil
}

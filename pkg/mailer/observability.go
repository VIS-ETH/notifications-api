package mailer

import (
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

type MailObservability struct {
	counter *metric.Int64Counter
}

var mo *MailObservability

func getMailObservability() (*MailObservability, error) {
	if mo != nil {
		return mo, nil
	}
	meter := otel.Meter("mailer-meter")
	counter, err := meter.Int64Counter(
		"mail_sender_total_mail_count",
		metric.WithDescription("Total mails sent by mailer"),
		metric.WithUnit("{call}"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTEL counter: %v", err)
	}

	mo = &MailObservability{
		counter: &counter,
	}

	return mo, nil
}

package dispatcher

import (
	"context"
	"fmt"
	"time"

	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/generated/sql"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

type DispatcherObservability struct {
	reg             metric.Registration
	attemptsCounter metric.Int64Counter
}

func initializeQueueObservability(querier sql.Querier) (*DispatcherObservability, error) {
	meter := otel.Meter("internal/dispatcher")

	statusGauge, err := meter.Int64ObservableGauge(
		"mail_status_count",
		metric.WithDescription("Current number of mails in each status"),
		metric.WithUnit("{mail}"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create status gauge: %w", err)
	}

	attemptsCounter, err := meter.Int64Counter(
		"mail_dispatch_attempts_total",
		metric.WithDescription("Total mail dispatch attempts, by resulting status"),
		metric.WithUnit("{attempt}"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create attempts counter: %w", err)
	}

	reg, err := meter.RegisterCallback(
		func(ctx context.Context, o metric.Observer) error {
			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			counts, err := querier.CountMailsByStatus(ctx)
			if err != nil {
				return fmt.Errorf("failed to count mails by status: %w", err)
			}

			for _, c := range counts {
				o.ObserveInt64(statusGauge, c.Count,
					metric.WithAttributes(attribute.String("status", string(c.Status))))
			}

			return nil
		},
		statusGauge,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to register status-count callback: %w", err)
	}

	return &DispatcherObservability{
		reg:             reg,
		attemptsCounter: attemptsCounter,
	}, nil
}

func (d *DispatcherObservability) RecordAttempt(ctx context.Context, status sql.MailStatus) {
	d.attemptsCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("status", string(status))))
}

func (d *DispatcherObservability) Close() error {
	if d.reg == nil {
		return nil
	}
	return d.reg.Unregister()
}

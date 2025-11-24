package internal

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/sirupsen/logrus"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/generated/sql"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal/database"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/pkg/mailer"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	checkInterval  = 1 * time.Minute
	deleteInterval = 1 * time.Hour
)

var (
	retryFailedIntervalMicroseconds = 1 * time.Hour.Microseconds()
	keepMailsForIntervalDays        = /* 2 weeks */ 14
)

// HandleMailQueue continuously watches the database (every minute) and
// tries to send any remaining mail (according to rate limit rules etc.)
func HandleMailQueue(ctx context.Context, mailSender *mailer.MailSender, queries *sql.Queries) error {
	logger := logrus.WithFields(logrus.Fields{
		"component": "queue-handler",
	})
	logger.Infof("Starting mail queue handler")

	for {
		for {
			empty, err := popAndHandleMail(ctx, logger, queries, mailSender)
			if err != nil {
				logger.Errorf("Failed to handle mail: %v", err)
				break
			}
			if empty {
				break
			}
		}
		select {
		case <-time.Tick(checkInterval):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func popAndHandleMail(ctx context.Context, logger *logrus.Entry, queries *sql.Queries, mailSender *mailer.MailSender) (bool, error) {
	poppedMails, err := queries.PopMailForProcessing(ctx, pgtype.Interval{
		Microseconds: retryFailedIntervalMicroseconds,
	})
	if err != nil {
		return false, fmt.Errorf("failed to query for mails, skipping: %v", err)
	}
	if len(poppedMails) > 1 {
		logger.Fatalf("Cannot handle more than 1 popped mail at a time, was %d", len(poppedMails))
	} else if len(poppedMails) == 0 {
		// nothing to do - skip and wait
		return true, nil
	}
	poppedMail := poppedMails[0]

	tr := otel.Tracer("notifications-api/queuehandler")
	ctx, span := tr.Start(ctx, "notifications-api.handle-queued-mail",
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(
			attribute.Int("mail.id", int(poppedMail.ID)),
		),
	)
	defer span.End()
	status := sql.MailStatusFailed

	defer func() {
		logger.Tracef("Setting mail status to %v", status)
		err = errors.Join(err, queries.SetMailStatus(ctx, sql.SetMailStatusParams{
			ID:     poppedMail.ID,
			Status: status,
		}))
		if err != nil {
			logger.Errorf("error in defer: %v", err)
		}
	}()

	queuedMail, err := database.DBEntityToMail(poppedMail)
	if err != nil {
		return false, fmt.Errorf("failed to convert mail back from SQL entity: %v", err)
	}
	err = mailSender.TransmitMail(ctx, queuedMail)
	if err != nil {
		return false, fmt.Errorf("failed to transmit mail, marking as failed: %v", err)
	}
	status = sql.MailStatusSent
	span.SetStatus(codes.Ok, "queued mail handled")

	return false, err
}

func DeleteOldMails(ctx context.Context, queries *sql.Queries) error {
	logger := logrus.WithFields(logrus.Fields{
		"component": "db-delete-mail-handler",
	})
	for {
		err := queries.DeleteOldMails(ctx, pgtype.Interval{
			Days: int32(keepMailsForIntervalDays),
		})
		if err != nil {
			logger.Errorf("failed to delete mails: %v", err)
		}

		select {
		case <-time.Tick(deleteInterval):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

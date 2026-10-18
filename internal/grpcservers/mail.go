package grpcservers

import (
	"context"
	"errors"
	"fmt"
	"net/mail"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/sirupsen/logrus"
	pb "gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/generated/pb/sip/notifications"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/generated/sql"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal/auth"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal/database"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/pkg/mailer"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

const (
	MessageIDLoggerField = "message-id"
	RPCMethodLoggerField = "rpc-method"

	MaxMailSizeBytes = 2 * 1024 * 1024 // 2 MB
)

func (s *MailServer) QueueMail(ctx context.Context, mailReq *pb.Mail) (*pb.QueueResponse, error) {
	sanitizedMail, err := s.preprocessIncomingMailRequest(ctx, mailReq)
	if err != nil {
		return nil, err
	}

	logger := s.logger.WithFields(logrus.Fields{
		MessageIDLoggerField: sanitizedMail.MessageID,
		RPCMethodLoggerField: "queue-mail",
	})

	if s.loggingOnly {
		mailResponse := &pb.MailResponse{
			MailId: sanitizedMail.MessageID,
		}

		logger.Infof("Logging-only mode: requested to send mail %+v", sanitizedMail)
		logger.Infof("Logging-only mode: return successful response to %+v", mailResponse)

		imf, err := sanitizedMail.GetInternetMessageFormat(true)
		if err != nil {
			s.logger.Errorf("Failed to get internet message format for logging: %v", err)
			return nil, status.Errorf(codes.Internal, "Failed to get internet message format for logging")
		}
		logger.Tracef("Logging-only mode: full message content: %q", imf)
		return &pb.QueueResponse{
			MailId: sanitizedMail.MessageID,
		}, nil
	}

	mailSQLEntity, err := database.MailToDBEntity(sanitizedMail)
	if err != nil {
		logger.Errorf("Failing to insert mail into DB (conversion): %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to convert mail to DB format!")
	}
	err = s.querier.CreateMail(ctx, sql.CreateMailParams{
		FromAddress:  mailSQLEntity.FromAddress,
		ReplyTo:      mailSQLEntity.ReplyTo,
		ToAddresses:  mailSQLEntity.ToAddresses,
		CcAddresses:  mailSQLEntity.CcAddresses,
		BccAddresses: mailSQLEntity.BccAddresses,
		ExtraHeaders: mailSQLEntity.ExtraHeaders,
		Subject:      mailSQLEntity.Subject,
		Body:         mailSQLEntity.Body,
		MessageID:    mailSQLEntity.MessageID,
		Status:       sql.MailStatusSubmitted,
		DateHeader:   pgtype.Timestamptz{Time: sanitizedMail.Date, Valid: !sanitizedMail.Date.IsZero()},
	})
	if err != nil {
		logger.Errorf("Failed to insert mail into queue: %v", err)
		return nil, status.Errorf(codes.Internal, "Cannot register mail into queue")
	}
	return &pb.QueueResponse{
		MailId: sanitizedMail.MessageID,
	}, nil
}

func (s *MailServer) SendMail(ctx context.Context, mailReq *pb.Mail) (*pb.MailResponse, error) {
	sanitizedMail, err := s.preprocessIncomingMailRequest(ctx, mailReq)
	if err != nil {
		return nil, err
	}

	logger := s.logger.WithFields(logrus.Fields{
		MessageIDLoggerField: sanitizedMail.MessageID,
		RPCMethodLoggerField: "send-mail",
	})

	if s.loggingOnly {
		mailResponse := &pb.MailResponse{
			MailId: sanitizedMail.MessageID,
		}

		logger.Infof("Logging-only mode: requested to send mail %+v", sanitizedMail)
		logger.Infof("Logging-only mode: return successful response to %+v", mailResponse)

		imf, err := sanitizedMail.GetInternetMessageFormat(true)
		if err != nil {
			s.logger.Errorf("Failed to get internet message format for logging: %v", err)
			return nil, status.Errorf(codes.Internal, "Failed to get internet message format for logging")
		}
		logger.Tracef("Logging-only mode: full message content: %q", imf)
		return mailResponse, nil
	}

	mailSQLEntity, err := database.MailToDBEntity(sanitizedMail)
	if err != nil {
		logger.Errorf("Failing to insert mail into DB (conversion): %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to convert mail to DB format!")
	}

	err = s.mailSender.TransmitMail(ctx, sanitizedMail)
	if err != nil {
		s.logger.Errorf("Failed to send mail: %v -  %v", *sanitizedMail, err)
		return nil, status.Errorf(codes.Internal, "Failed to send mail via SMTP")
	}

	err = s.querier.CreateMail(ctx, sql.CreateMailParams{
		FromAddress:  mailSQLEntity.FromAddress,
		ReplyTo:      mailSQLEntity.ReplyTo,
		ToAddresses:  mailSQLEntity.ToAddresses,
		CcAddresses:  mailSQLEntity.CcAddresses,
		BccAddresses: mailSQLEntity.BccAddresses,
		ExtraHeaders: mailSQLEntity.ExtraHeaders,
		Subject:      mailSQLEntity.Subject,
		Body:         mailSQLEntity.Body,
		MessageID:    mailSQLEntity.MessageID,
		DateHeader:   pgtype.Timestamptz{Time: sanitizedMail.Date, Valid: !sanitizedMail.Date.IsZero()},
		Status:       sql.MailStatusSent,
	})
	// To avoid retry behavior by application or other things - report success as mail was actually send
	if err != nil {
		logger.Errorf("Failed to insert in the database: %v", err)
		err = nil
	}

	return &pb.MailResponse{
		MailId: string(sanitizedMail.MessageID),
	}, err
}

func (s *MailServer) SendDryRun(ctx context.Context, mailReq *pb.Mail) (*pb.Mail, error) {
	sanitizedMail, err := s.preprocessIncomingMailRequest(ctx, mailReq)
	if err != nil {
		return nil, err
	}

	logger := s.logger.WithFields(logrus.Fields{
		MessageIDLoggerField: sanitizedMail.MessageID,
		RPCMethodLoggerField: "dryrun-mail",
	})

	mailSQLEntity, err := database.MailToDBEntity(sanitizedMail)
	if err != nil {
		logger.Errorf("Failing to insert mail into DB (conversion): %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to convert mail to DB format!")
	}

	err = s.mailSender.TransmitMail(ctx, sanitizedMail)
	if err != nil {
		s.logger.Errorf("Failed to send mail: %v -  %v", *sanitizedMail, err)
		return nil, status.Errorf(codes.Internal, "Failed to send mail via SMTP")
	}

	err = s.querier.CreateMail(ctx, sql.CreateMailParams{
		FromAddress:  mailSQLEntity.FromAddress,
		ReplyTo:      mailSQLEntity.ReplyTo,
		ToAddresses:  mailSQLEntity.ToAddresses,
		CcAddresses:  mailSQLEntity.CcAddresses,
		BccAddresses: mailSQLEntity.BccAddresses,
		ExtraHeaders: mailSQLEntity.ExtraHeaders,
		Subject:      mailSQLEntity.Subject,
		Body:         mailSQLEntity.Body,
		MessageID:    mailSQLEntity.MessageID,
		DateHeader:   pgtype.Timestamptz{Time: sanitizedMail.Date, Valid: !sanitizedMail.Date.IsZero()},
		Status:       sql.MailStatusSent,
	})
	// To avoid retry behavior by application or other things - report success as mail was actually send
	if err != nil {
		logger.Errorf("Failed to insert in the database: %v", err)
		err = nil
	}

	copy := proto.CloneOf(mailReq)
	// copy.From =
	return copy, err
}

func (s *MailServer) preprocessIncomingMailRequest(ctx context.Context, mailReq *pb.Mail) (*mailer.Mail, error) {
	s.logger.Trace("Generating UUID for message...")

	messageUUID, err := uuid.NewV7()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to generate uuid: %v", err)
	}
	messageID := fmt.Sprintf("%s@%s", messageUUID.String(), s.mailSender.MessageIDSuffix())

	if len(mailReq.To) == 0 && len(mailReq.Cc) == 0 && len(mailReq.Bcc) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "Provided message had no receiver! To/Cc/Bcc all empty!")
	}

	if mailReq.Subject == "" || mailReq.BodyOneof == nil {
		return nil, status.Errorf(codes.InvalidArgument, "Provided message had no content, pass subject and body!")
	}

	logger := s.logger.WithFields(logrus.Fields{
		"message-id": messageID,
	})

	claims, err := auth.GetClaimsFromEnrichedGrpcCtx(ctx)
	if err != nil {
		if s.unauthenticated {
			logger.Debugf("Unauthenticated mode: running request with failed authorization check: %v", err)
		} else {
			return nil, err
		}
	}

	logger.Trace("Transforming & sanitizing proto mail format to internal formats...")

	var sender mail.Address
	if mailReq == nil || mailReq.From == nil || mailReq.From.GetMailAddress() == nil {
		sender = s.mailSender.GetSender(mail.Address{})
	} else {
		desiredSender := mail.Address{
			Name:    mailReq.From.GetMailAddress().Name,
			Address: mailReq.From.GetMailAddress().Address,
		}
		sender = s.mailSender.GetSender(desiredSender)
	}

	validatedMail, err := pbMailToValidatedMail(mailReq, sender)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "Provided message was invalid: %v", err)
	}

	logger.Tracef("Setting message ID")
	validatedMail.MessageID = messageID

	err = s.checkSendPermission(claims, mailReq, validatedMail)
	if err != nil {
		if s.unauthenticated {
			logger.Debugf("Unauthenticated mode: some values are not permitted... %v", err)
		} else {
			return nil, status.Errorf(codes.PermissionDenied, "Cannot send mails: %v", err)
		}
	}

	return validatedMail, nil
}

func (s *MailServer) checkSendPermission(claims *auth.CustomClaims, mailReq *pb.Mail, message *mailer.Mail) error {
	var permErrors []error

	if !claims.CanMail() {
		permErrors = append(permErrors, errors.New("not allowed to send mails via Notifications API"))
	}

	// perform check on filled-in defaults
	if !claims.IsSenderAllowed(message.From.Address) {
		permErrors = append(permErrors, fmt.Errorf("not allowed to send from address: %s", message.From.Address))
	}

	// perform check only on request
	for _, header := range mailReq.ExtraHeaders {
		if header == nil {
			permErrors = append(permErrors, fmt.Errorf("extra header was nil"))
			continue
		}

		if !claims.IsHeaderAllowed(header.Key) {
			permErrors = append(permErrors, fmt.Errorf("not allowed to set extra header: %s", header.Key))
		}
	}

	// check size of mail request
	msgContent, err := message.GetInternetMessageFormat(false)
	if err != nil {
		return fmt.Errorf("failed to get internet message format for size check: %v", err)
	}
	if len(msgContent) > MaxMailSizeBytes {
		permErrors = append(permErrors, fmt.Errorf(
			"mail size exceeds maximum allowed size of %d bytes: %d bytes", MaxMailSizeBytes, len(msgContent)))
	}

	return errors.Join(permErrors...)
}

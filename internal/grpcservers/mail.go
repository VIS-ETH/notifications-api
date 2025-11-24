package grpcservers

import (
	"context"
	"errors"
	"fmt"
	"net/mail"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	pb "gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/generated/pb/sip/notifications"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/generated/sql"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal/auth"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal/database"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/pkg/mailer"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	MessageIDLoggerField = "message-id"
	RPCMethodLoggerField = "rpc-method"
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

	if *s.loggingOnly {
		mailResponse := &pb.MailResponse{
			MailId: sanitizedMail.MessageID,
		}
		logger.Infof("Logging-only mode: requested to send mail %+v", sanitizedMail)
		logger.Infof("Logging-only mode: return successful response to %+v", mailResponse)
		logger.Tracef("Logging-only mode: full message content: %s", sanitizedMail.GetMessageContent())
		return &pb.QueueResponse{
			MailId: sanitizedMail.MessageID,
		}, nil
	}

	mailSQLEntity, err := database.MailToDBEntity(sanitizedMail)
	if err != nil {
		logger.Errorf("Failing to insert mail into DB (conversion): %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to convert mail to DB format!")
	}

	err = s.queries.CreateMail(ctx, sql.CreateMailParams{
		FromAddress:  mailSQLEntity.FromAddress,
		ReplyTo:      mailSQLEntity.ReplyTo,
		ToAddresses:  mailSQLEntity.ToAddresses,
		CcAddresses:  mailSQLEntity.CcAddresses,
		BccAddresses: mailSQLEntity.BccAddresses,
		ExtraHeaders: mailSQLEntity.ExtraHeaders,
		Subject:      mailSQLEntity.Subject,
		Body:         mailSQLEntity.Body,
		MessageID:    mailSQLEntity.MessageID,
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

	if *s.loggingOnly {
		mailResponse := &pb.MailResponse{
			MailId: sanitizedMail.MessageID,
		}
		logger.Infof("Logging-only mode: requested to send mail %+v", sanitizedMail)
		logger.Infof("Logging-only mode: return successful response to %+v", mailResponse)
		logger.Tracef("Logging-only mode: full message content: %s", sanitizedMail.GetMessageContent())
		return mailResponse, nil
	}

	err = s.mailSender.TransmitMail(ctx, sanitizedMail)

	return &pb.MailResponse{
		MailId: string(sanitizedMail.MessageID),
	}, err
}

func (s *MailServer) preprocessIncomingMailRequest(ctx context.Context, mailReq *pb.Mail) (*mailer.Mail, error) {
	s.logger.Trace("Generating UUID for message...")

	messageUUID, err := uuid.NewV7()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to generate uuid: %v", err)
	}
	messageID := fmt.Sprintf("%s@%s", messageUUID.String(), "mail-api-vis")

	logger := s.logger.WithFields(logrus.Fields{
		"message-id": messageID,
	})

	claims, err := auth.GetClaimsFromEnrichedGrpcCtx(ctx)
	if err != nil {
		if *s.unauthenticated {
			logger.Debugf("Unauthenticated mode: running request with failed authorization check: %v", err)
		} else {
			return nil, err
		}
	}

	logger.Trace("Transforming & sanitizing proto mail format to internal formats...")

	sanitizedMail, err := pbMailToSanitizedMail(mailReq, s.mailSender.DefaultSenderAddress())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "Provided message was invalid: %v", err)
	}

	logger.Tracef("Setting message ID")
	sanitizedMail.MessageID = messageID

	err = s.checkSendPermission(claims, sanitizedMail)
	if err != nil {
		if *s.unauthenticated {
			logger.Debugf("Unauthenticated mode: some values are not permitted... %v", err)
		} else {
			return nil, status.Errorf(codes.PermissionDenied, "Cannot send mails: %v", err)
		}
	}

	return sanitizedMail, nil
}

func (s *MailServer) checkSendPermission(claims *auth.CustomClaims, message *mailer.Mail) error {
	var permErrors []error

	if !claims.CanMail() {
		permErrors = append(permErrors, errors.New("not allowed to send mails via Notifications API"))
	}

	// Everyone is allowed to send as the default - if allowed to send at all
	isSenderAllowed := (message.From.Address == s.mailSender.DefaultSenderAddress()) ||
		claims.IsSenderAllowed(&message.From.Address)

	if !isSenderAllowed {
		permErrors = append(permErrors, fmt.Errorf("not allowed to send from address: %s", message.From.Address))
	}

	// if len(message.To) > 500 {

	//}
	return errors.Join(permErrors...)
}

func pbMailToSanitizedMail(mailReq *pb.Mail, defaultSender string) (*mailer.Mail, error) {
	if mailReq.From == nil {
		mailReq.From = &pb.MailAddress{
			Address: &pb.MailAddress_MailAddress{
				MailAddress: &pb.MailAddress_Address{
					Name:    "Serviceaccount Mail API",
					Address: defaultSender,
				},
			},
		}
	}

	fromAddr, err := transformAddressFormatsToPlainAddress(mailReq.From)
	if err != nil {
		return nil, fmt.Errorf("sender address could not be converted and sanitized")
	}

	transformedReplyToAddresses, err := transformAddressLists(mailReq.ReplyTo)
	if err != nil {
		return nil, fmt.Errorf("could not convert 'reply-to' address list: %v", err)
	}

	transformedToAddresses, err := transformAddressLists(mailReq.To)
	if err != nil {
		return nil, fmt.Errorf("could not convert 'to' address list: %v", err)
	}
	transformedCcAddresses, err := transformAddressLists(mailReq.Cc)
	if err != nil {
		return nil, fmt.Errorf("could not convert 'cc' address list: %v", err)
	}
	transformedBccAddresses, err := transformAddressLists(mailReq.Bcc)
	if err != nil {
		return nil, fmt.Errorf("could not convert 'bcc' address list: %v", err)
	}

	if len(mailReq.ExtraHeader) > 0 {
		return nil, errors.New("extra_header not yet supported")
	}

	var bodyContent string
	var extraHeaders map[string][]string

	switch body := mailReq.BodyOneof.(type) {
	case *pb.Mail_PlainText:
		bodyContent = body.PlainText
	case *pb.Mail_MultipartBody:
		// extraHeaders["Content-Type"] = []string{"multipart"}
		return nil, errors.New("multipart mail not supported")
	}

	return &mailer.Mail{
		From:         fromAddr,
		ReplyTo:      transformedReplyToAddresses,
		To:           transformedToAddresses,
		Cc:           transformedCcAddresses,
		Bcc:          transformedBccAddresses,
		ExtraHeaders: extraHeaders,
		Subject:      mailReq.Subject,
		Body:         bodyContent,
	}, nil
}

func transformAddressLists(addresses []*pb.MailAddress) ([]*mail.Address, error) {
	var addressList []*mail.Address
	for i, addr := range addresses {
		transformedAddr, err := transformAddressFormatsToPlainAddress(addr)
		if err != nil {
			return nil, fmt.Errorf("failed to transform address %d (%s) in address list", i, addr)
		}
		addressList = append(addressList, transformedAddr)
	}
	return addressList, nil
}

func transformAddressFormatsToPlainAddress(addr *pb.MailAddress) (*mail.Address, error) {
	if addr == nil {
		return nil, errors.New("mail address was nil")
	}
	switch addr := addr.Address.(type) {
	case *pb.MailAddress_MailAddress:
		// Sanitization:
		// - Check that address itself is valid
		// - Check that as a whole, it is also still valid
		parsedAddress, err := mail.ParseAddress(addr.MailAddress.Address)
		if err != nil {
			return nil, fmt.Errorf("failed to parse address itself of %s: %v", addr.MailAddress.Address, err)
		}
		if parsedAddress.Address != addr.MailAddress.Address {
			return nil, fmt.Errorf(
				"parsed plain address differred from supposed-to-be-plain address: '%s' vs '%s'",
				parsedAddress.Address, addr.MailAddress.Address)
		}
		stdMailAddr := mail.Address{
			Name:    addr.MailAddress.Name,
			Address: addr.MailAddress.Address,
		}
		if _, err := mail.ParseAddress(stdMailAddr.String()); err != nil {
			return nil, fmt.Errorf("a funny sanity check failed... %v", err)
		}
		return &stdMailAddr, nil
	case *pb.MailAddress_VsethUserId:
		return nil, fmt.Errorf("VSETH user id as mail address not yet supported")
	default:
		return nil, fmt.Errorf("proto mail address could not be converted")
	}
}

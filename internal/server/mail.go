package server

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"net/smtp"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	pb "gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/generated/pb/sip/notifications"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal/auth"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Mail struct {
	From         *mail.Address
	ReplyTo      []*mail.Address
	To           []*mail.Address
	Cc           []*mail.Address
	Bcc          []*mail.Address
	ExtraHeaders map[string][]string
	Subject      string
	Body         string
}

func (s *NotificationsServer) SendMail(ctx context.Context, mailReq *pb.Mail) (*pb.MailResponse, error) {
	s.mailConfig.logger.Trace("Generating UUID for message...")

	messageUUID, err := uuid.NewV7()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to generate uuid: %v", err)
	}
	messageID := fmt.Sprintf("%s@%s", messageUUID.String(), "mail-api-vis")

	logger := s.mailConfig.logger.WithFields(logrus.Fields{
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

	sanitizedMail, err := pbMailToSanitizedMail(mailReq, s.mailConfig.defaultMailSenderAddress)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "Provided message was invalid: %v", err)
	}

	err = s.checkSendPermission(claims, sanitizedMail)
	if err != nil {
		if *s.unauthenticated {
			logger.Debugf("Unauthenticated mode: some values are not permitted... %v", err)
		} else {
			return nil, status.Errorf(codes.PermissionDenied, "Cannot send mails: %v", err)
		}
	}

	if *s.loggingOnly {
		mailResponse := &pb.MailResponse{
			MailId: messageID,
		}
		logger.Infof("Logging-only mode: requested to send mail %+v", sanitizedMail)
		logger.Infof("Logging-only mode: return successful response to %+v", mailResponse)
		logger.Tracef("Logging-only mode: full message content: %s", getMessageContent(sanitizedMail, messageID))
		return mailResponse, nil
	}

	err = s.transmitMail(sanitizedMail, messageID, logger)

	return &pb.MailResponse{
		MailId: string(messageID),
	}, err
}

func (s *NotificationsServer) checkSendPermission(claims *auth.CustomClaims, message *Mail) error {
	var permErrors []error

	if !claims.CanMail() {
		permErrors = append(permErrors, errors.New("not allowed to send mails via Notifications API"))
	}

	// Everyone is allowed to send as the default - if allowed to send at all
	isSenderAllowed := (message.From.Address == s.mailConfig.defaultMailSenderAddress) ||
		claims.IsSenderAllowed(&message.From.Address)

	if !isSenderAllowed {
		permErrors = append(permErrors, fmt.Errorf("not allowed to send from address: %s", message.From.Address))
	}

	// if len(message.To) > 500 {

	//}
	return errors.Join(permErrors...)
}

func getMessageContent(sanitizedMail *Mail, messageID string) string {
	var messageBuilder strings.Builder
	messageBuilder.WriteString(fmt.Sprintf("Date: %s\n", time.Now().Format(time.RFC822Z)))
	messageBuilder.WriteString(fmt.Sprintf("From: %s\n", sanitizedMail.From.String()))
	messageBuilder.WriteString(fmt.Sprintf("Message-ID: <%s>\n", messageID))
	messageBuilder.WriteString(fmt.Sprintf("Subject: %s\n", sanitizedMail.Subject))

	if len(sanitizedMail.To) > 0 {
		var toAddresses []string
		for _, toAddr := range sanitizedMail.To {
			toAddresses = append(toAddresses, toAddr.String())
		}
		messageBuilder.WriteString(fmt.Sprintf("To: %s\n", strings.Join(toAddresses, "\n ,")))
	}

	if len(sanitizedMail.Cc) > 0 {
		var ccAddresses []string
		for _, ccAddr := range sanitizedMail.Cc {
			ccAddresses = append(ccAddresses, ccAddr.String())
		}
		messageBuilder.WriteString(fmt.Sprintf("Cc: %s\n", strings.Join(ccAddresses, "\n ,")))
	}

	if len(sanitizedMail.ReplyTo) > 0 {
		var replyToAddresses []string
		for _, replyToAddr := range sanitizedMail.Cc {
			replyToAddresses = append(replyToAddresses, replyToAddr.String())
		}
		messageBuilder.WriteString(fmt.Sprintf("Reply-To: %s\n", strings.Join(replyToAddresses, "\n ,")))
	}

	messageBuilder.WriteString("\n")
	messageBuilder.WriteString(sanitizedMail.Body)

	messageContent := messageBuilder.String()
	messageContent = strings.ReplaceAll(messageContent, "\r\n", "\n")
	messageContent = strings.ReplaceAll(messageContent, "\n", "\r\n")

	return messageContent
}

func (s *NotificationsServer) transmitMail(sanitizedMail *Mail, messageID string, logger *logrus.Entry) error {
	logger.Trace("Establishing SMTP connection")

	client, err := smtp.Dial(s.mailConfig.smtpEndpoint)
	if err != nil {
		return fmt.Errorf("failed to dial smtp server and retrieve client: %v", err)
	}
	defer func() {
		err = client.Close()
		if err != nil {
			err = status.Errorf(codes.Aborted, "Failed to close smtp client: %v", err)
		}
	}()

	if err != nil {
		return status.Error(codes.Internal, "failed to connect mail server")
	}

	logger.Trace("SMTP From")

	if err := client.Mail(sanitizedMail.From.Address); err != nil {
		logger.Errorf("Failed to mail from %s: %v", sanitizedMail.From.Address, err)
		return status.Errorf(codes.Aborted, "could not start sending mail with from mail address %s", sanitizedMail.From.Address)
	}

	logger.Trace("Setting SMTP recipients")

	recipients := slices.Concat(sanitizedMail.To, sanitizedMail.Cc, sanitizedMail.Bcc)
	for i, to := range recipients {
		if err := client.Rcpt(to.Address); err != nil {
			logger.Errorf("Failed to add recipient %d (%s): %v", i, to.Address, err)
			return status.Errorf(codes.Aborted, "Could not add recipient %d: %s", i, to.Address)
		}
	}

	logger.Trace("Transmitting data...")

	wc, err := client.Data()
	if err != nil {
		return status.Errorf(codes.Aborted, "Could not start sending data: %v", err)
	}
	defer func() {
		err = wc.Close()
		if err != nil {
			err = status.Errorf(codes.Aborted, "Failed to close data writer: %v", err)
		}
	}()

	_, err = wc.Write([]byte(getMessageContent(sanitizedMail, messageID)))
	if err != nil {
		return status.Errorf(codes.Aborted, "Failed to write message content: %v", err)
	}

	logger.Trace("Successfully written message")
	return nil
}

func pbMailToSanitizedMail(mailReq *pb.Mail, defaultSender string) (*Mail, error) {
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

	return &Mail{
		From:    fromAddr,
		ReplyTo: transformedReplyToAddresses,
		To:      transformedToAddresses,
		Cc:      transformedCcAddresses,
		Bcc:     transformedBccAddresses,
		Subject: mailReq.Subject,
		Body:    mailReq.Body,
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
		return nil, fmt.Errorf("Mail address was nil")
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

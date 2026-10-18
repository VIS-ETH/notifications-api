package smtpproxy

import (
	"fmt"

	pb "gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/generated/pb/sip/notifications"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/pkg/mailer"
)

func buildGrpcRequestFromMail(mail *mailer.Mail) (*pb.Mail, error) {
	to := make([]*pb.MailAddress, len(mail.To))
	for i, addr := range mail.To {
		to[i] = internal.MakePbAddress(addr.Name, addr.Address)
	}
	cc := make([]*pb.MailAddress, len(mail.Cc))
	for i, addr := range mail.Cc {
		cc[i] = internal.MakePbAddress(addr.Name, addr.Address)
	}
	bcc := make([]*pb.MailAddress, len(mail.Bcc))
	for i, addr := range mail.Bcc {
		bcc[i] = internal.MakePbAddress(addr.Name, addr.Address)
	}
	replyTo := make([]*pb.MailAddress, len(mail.ReplyTo))
	for i, addr := range mail.ReplyTo {
		replyTo[i] = internal.MakePbAddress(addr.Name, addr.Address)
	}

	var extraHeaders []*pb.ExtraHeader
	for _, vs := range mail.Headers {
		extraHeaders = append(extraHeaders, &pb.ExtraHeader{Key: vs.Key, Value: vs.Value})
	}

	rawMail, ok := mail.Body.(mailer.Binary)
	if !ok {
		return nil, fmt.Errorf("mail body is not of type Binary, got %T", mail.Body)
	}

	pbMail := &pb.Mail{
		Subject:      mail.Subject,
		From:         internal.MakePbAddress(mail.From.Name, mail.From.Address),
		To:           to,
		Cc:           cc,
		Bcc:          bcc,
		ReplyTo:      replyTo,
		ExtraHeaders: extraHeaders,
		ContentType:  &mail.ContentType,
		BodyOneof: &pb.Mail_RawBody{
			RawBody: rawMail.Content,
		},
	}

	return pbMail, nil
}

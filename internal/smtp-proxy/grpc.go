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

	pbMail := &pb.Mail{
		Subject:      mail.Subject,
		From:         internal.MakePbAddress(mail.From.Name, mail.From.Address),
		To:           to,
		Cc:           cc,
		Bcc:          bcc,
		ReplyTo:      replyTo,
		ExtraHeaders: extraHeaders,
	}
	if mail.ContentType != "" {
		pbMail.ContentType = &mail.ContentType
	}

	switch body := mail.Body.(type) {
	case mailer.Text:
		pbMail.BodyOneof = &pb.Mail_PlainText{PlainText: body.Content}
	case mailer.Binary:
		pbMail.BodyOneof = &pb.Mail_RawBody{RawBody: body.Content}
	case mailer.Multipart:
		pbMimeBody, err := transformMultipartToProto(body)
		if err != nil {
			return nil, fmt.Errorf("error while transforming multipart: %v", err)
		}
		pbMail.BodyOneof = &pb.Mail_MultipartBody{MultipartBody: pbMimeBody}
	default:
		return nil, fmt.Errorf("unsupported mail body type: %T", mail.Body)
	}

	return pbMail, nil
}

func transformMultipartToProto(mp mailer.Multipart) (*pb.MimeBody, error) {
	parts := make([]*pb.MimePart, len(mp.Children))
	for i, child := range mp.Children {
		pbPart, err := transformPartToProto(child.Part, child.ContentType, child.Headers)
		if err != nil {
			return nil, err
		}
		parts[i] = pbPart
	}
	return &pb.MimeBody{Parts: parts}, nil
}

func transformPartToProto(part mailer.Part, contentType string, headers []mailer.Header) (*pb.MimePart, error) {
	var pbHeaders []*pb.ExtraHeader
	for _, h := range headers {
		pbHeaders = append(pbHeaders, &pb.ExtraHeader{Key: h.Key, Value: h.Value})
	}

	var ctPtr *string
	if contentType != "" {
		ctPtr = &contentType
	}

	pbPart := &pb.MimePart{
		ContentType: ctPtr,
		Headers:     pbHeaders,
	}

	switch p := part.(type) {
	case mailer.Text:
		pbPart.ContentOneof = &pb.MimePart_Content{Content: p.Content}
	case mailer.Binary:
		pbPart.ContentOneof = &pb.MimePart_ContentBytes{ContentBytes: p.Content}
	case mailer.Multipart:
		nestedBody, err := transformMultipartToProto(p)
		if err != nil {
			return nil, fmt.Errorf("error while transforming multipart: %v", err)
		}
		pbPart.ContentOneof = &pb.MimePart_Nested{Nested: nestedBody}
	default:
		return nil, fmt.Errorf("unsupported part type: %T", part)
	}

	return pbPart, nil
}

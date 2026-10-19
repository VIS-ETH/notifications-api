package grpcservers

import (
	"errors"
	"fmt"
	"mime"
	"net/mail"
	"time"

	pb "gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/generated/pb/sip/notifications"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/pkg/mailer"
)

func pbMailToValidatedMail(mailReq *pb.Mail, from mail.Address) (*mailer.Mail, error) {
	mailReq.From = &pb.MailAddress{
		Address: &pb.MailAddress_MailAddress{
			MailAddress: &pb.MailAddress_Address{
				Name:    from.Name,
				Address: from.Address,
			},
		},
	}

	fromAddr, err := transformAddressFormatsToPlainAddress(mailReq.From)
	if err != nil {
		return nil, fmt.Errorf("sender address could not be converted and sanitized: %v", err)
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

	var headers []mailer.Header
	for _, values := range mailReq.ExtraHeaders {
		if values == nil {
			return nil, fmt.Errorf("extra header was nil")
		}

		err := mailer.ValidateHeaderKey(values.Key)
		if err != nil {
			return nil, fmt.Errorf("extra header key %q was invalid: %v", values.Key, err)
		}
		if mailer.IsHeaderKeyReserved(values.Key) {
			return nil, fmt.Errorf("extra header key %q is reserved and cannot be set", values.Key)
		}
	}

	if mailReq.ContentType != nil {
		_, _, parseMediaTypeErr := mime.ParseMediaType(*mailReq.ContentType)
		if parseMediaTypeErr != nil {
			return nil, fmt.Errorf("failed to parse content type header %q: %v", *mailReq.ContentType, parseMediaTypeErr)
		}
	}
	var bodyContent string
	var mailBody mailer.Part

	switch body := mailReq.BodyOneof.(type) {
	case *pb.Mail_PlainText:
		if mailReq.ContentType != nil {
			bodyContent = *mailReq.ContentType
		} else {
			bodyContent = "text/plain; charset=utf-8"
		}
		mailBody = mailer.Text{
			Content: body.PlainText,
		}
	case *pb.Mail_SimpleMail:
		if mailReq.ContentType != nil {
			return nil, fmt.Errorf("content type header is not allowed to be set for simple mail body")
		}
		if body.SimpleMail.PlainText == nil && body.SimpleMail.Html == nil {
			return nil, fmt.Errorf("simple mail body must have at least one of plain text or html set")
		}
		if body.SimpleMail.Html == nil {
			bodyContent = "text/plain; charset=utf-8"
			mailBody = mailer.Text{Content: body.SimpleMail.GetPlainText()}
		} else if body.SimpleMail.PlainText == nil {
			bodyContent = "text/html; charset=utf-8"
			mailBody = mailer.Text{Content: body.SimpleMail.GetHtml()}
		} else {
			bodyContent = "multipart/alternative"
			mailBody = mailer.Multipart{
				Children: []mailer.Child{
					{
						ContentType: "text/plain; charset=utf-8",
						Part:        mailer.Text{Content: body.SimpleMail.GetPlainText()},
					},
					{
						ContentType: "text/html; charset=utf-8",
						Part:        mailer.Text{Content: body.SimpleMail.GetHtml()},
					},
				},
			}
		}
		if len(body.SimpleMail.Attachments) > 0 {
			allChildren := make([]mailer.Child, 0, 1+len(body.SimpleMail.Attachments))
			allChildren = append(allChildren, mailer.Child{
				ContentType: bodyContent,
				Part:        mailBody,
			})

			for _, attachment := range body.SimpleMail.Attachments {
				if attachment == nil {
					return nil, fmt.Errorf("attachment was nil")
				}
				contentType := "application/octet-stream"
				if attachment.ContentType != nil {
					contentType = *attachment.ContentType
				}
				// TODO: Content-ID, content-disposition, blabla
				allChildren = append(allChildren, mailer.Child{
					ContentType: contentType,
					Part: mailer.Binary{
						Content: attachment.Content,
					},
				})
			}
			bodyContent = "multipart/mixed"
			mailBody = mailer.Multipart{
				Children: allChildren,
			}
		}
	case *pb.Mail_MultipartBody:
		if mailReq.ContentType != nil {
			bodyContent = *mailReq.ContentType
		} else {
			bodyContent = "multipart/mixed"
		}
		transformedMultipart, err := transformPbMimeBodyToMailerBody(body.MultipartBody)
		if err != nil {
			return nil, fmt.Errorf("failed to transform multipart body: %v", err)
		}
		mailBody = transformedMultipart
	case *pb.Mail_RawBody:
		if mailReq.ContentType != nil {
			bodyContent = *mailReq.ContentType
		} else {
			bodyContent = "application/octet-stream"
		}
		mailBody = mailer.Binary{
			Content: body.RawBody,
		}
	default:
		return nil, fmt.Errorf("mail body was not set or was of unknown type: %T", mailReq.BodyOneof)
	}

	m := &mailer.Mail{
		From:        fromAddr,
		Subject:     mailReq.Subject,
		Date:        time.Now(),
		ContentType: bodyContent,

		ReplyTo: transformedReplyToAddresses,
		To:      transformedToAddresses,
		Cc:      transformedCcAddresses,
		Bcc:     transformedBccAddresses,

		Headers: headers,
		Body:    mailBody,
	}
	err = m.ValidateMail()
	if err != nil {
		return nil, fmt.Errorf("mail was invalid after transformation: %v", err)
	}

	return m, nil
}

func transformMimePartsToMailerParts(mimePart *pb.MimePart) (mailer.Part, error) {
	if mimePart == nil {
		return nil, fmt.Errorf("mime part was nil")
	}
	switch content := mimePart.ContentOneof.(type) {
	case *pb.MimePart_Content:
		return mailer.Text{
			Content: content.Content,
		}, nil
	case *pb.MimePart_ContentBytes:
		return mailer.Binary{
			Content: content.ContentBytes,
		}, nil
	case *pb.MimePart_Nested:
		return transformPbMimeBodyToMailerBody(content.Nested)
	}
	return nil, fmt.Errorf("unknown mime part content type: %T", mimePart.ContentOneof)
}

func transformPbMimeBodyToMailerBody(mimeBody *pb.MimeBody) (mailer.Multipart, error) {
	if mimeBody == nil {
		return mailer.Multipart{}, fmt.Errorf("mime body was nil")
	}
	children := make([]mailer.Child, 0, len(mimeBody.Parts))
	for _, part := range mimeBody.Parts {
		contentType := ""
		if part.ContentType != nil {
			contentType = *part.ContentType
		} else {
			switch part.ContentOneof.(type) {
			case *pb.MimePart_Content:
				contentType = "text/plain; charset=utf-8"
			case *pb.MimePart_ContentBytes:
				contentType = "application/octet-stream"
			case *pb.MimePart_Nested:
				contentType = "multipart/mixed"
			}
		}
		trasformedPart, err := transformMimePartsToMailerParts(part)
		if err != nil {
			return mailer.Multipart{}, fmt.Errorf("failed to transform child mime part: %v", err)
		}
		headers := make([]mailer.Header, 0, len(part.Headers))
		for _, header := range part.Headers {
			if header == nil {
				return mailer.Multipart{}, fmt.Errorf("mime part header was nil")
			}
			err := mailer.ValidateHeaderKey(header.Key)
			if err != nil {
				return mailer.Multipart{}, fmt.Errorf("mime part header key %q was invalid: %v", header.Key, err)
			}
			headers = append(headers, mailer.Header{
				Key:   header.Key,
				Value: header.Value,
			})
		}
		children = append(children, mailer.Child{
			ContentType: contentType,
			Headers:     headers,
			Part:        trasformedPart,
		})
	}

	return mailer.Multipart{
		Children: children,
	}, nil
}

func transformAddressLists(addresses []*pb.MailAddress) ([]mail.Address, error) {
	var addressList []mail.Address
	for i, addr := range addresses {
		transformedAddr, err := transformAddressFormatsToPlainAddress(addr)
		if err != nil {
			return nil, fmt.Errorf("failed to transform address %d (%s) in address list", i, addr)
		}
		addressList = append(addressList, transformedAddr)
	}
	return addressList, nil
}

func transformAddressFormatsToPlainAddress(addr *pb.MailAddress) (mail.Address, error) {
	if addr == nil {
		return mail.Address{}, errors.New("mail address was nil")
	}
	switch addr := addr.Address.(type) {
	case *pb.MailAddress_MailAddress:
		// Sanitization:
		// - Check that address itself is valid
		// - Check that as a whole, it is also still valid
		parsedAddress, err := mail.ParseAddress(addr.MailAddress.Address)
		if err != nil {
			return mail.Address{}, fmt.Errorf("failed to parse address itself of %s: %v", addr.MailAddress.Address, err)
		}
		if parsedAddress.Address != addr.MailAddress.Address {
			return mail.Address{}, fmt.Errorf(
				"parsed plain address differred from supposed-to-be-plain address: '%s' vs '%s'",
				parsedAddress.Address, addr.MailAddress.Address,
			)
		}
		stdMailAddr := mail.Address{
			Name:    addr.MailAddress.Name,
			Address: addr.MailAddress.Address,
		}
		if _, err := mail.ParseAddress(stdMailAddr.String()); err != nil {
			return mail.Address{}, fmt.Errorf("a funny sanity check failed... %v", err)
		}
		return stdMailAddr, nil
	case *pb.MailAddress_VsethUserId:
		return mail.Address{}, fmt.Errorf("VSETH user id as mail address not yet supported")
	default:
		return mail.Address{}, fmt.Errorf("proto mail address could not be converted")
	}
}

package database

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"

	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/generated/sql"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/pkg/mailer"
)

type bodyV1 struct {
	V    int    `json:"v"`
	Root partV1 `json:"root"`
}

type partV1 struct {
	Headers     []headerV1 `json:"h,omitempty"`
	ContentType string     `json:"ct"`
	Text        string     `json:"text,omitempty"`
	Data        []byte     `json:"data,omitempty"`
	Children    []partV1   `json:"children,omitempty"`
}

type headerV1 struct {
	Key   string `json:"k"`
	Value string `json:"v"`
}

func buildMailBodyDbObject(p mailer.Part, root *partV1) error {
	switch b := p.(type) {
	case mailer.Text:
		root.Text = b.Content
	case mailer.Binary:
		root.Data = b.Content
	case mailer.Multipart:
		children := make([]partV1, len(b.Children))
		for _, child := range b.Children {
			headerList := make([]headerV1, len(child.Headers))
			for _, h := range child.Headers {
				headerList = append(headerList, headerV1{Key: h.Key, Value: h.Value})
			}
			childPart := partV1{
				ContentType: child.ContentType,
				Headers:     headerList,
			}
			if err := buildMailBodyDbObject(child.Part, &childPart); err != nil {
				return fmt.Errorf("failed to build child part: %v", err)
			}
			children = append(children, childPart)
		}
		root.Children = children
	default:
		return fmt.Errorf("unsupported body type: %T", p)
	}
	return nil
}

func reconstructMailBodyFromDbObject(root partV1) (mailer.Part, error) {
	hasChildren := len(root.Children) > 0
	hasText := root.Text != ""
	hasData := len(root.Data) > 0

	var p mailer.Part

	if hasChildren {
		childParts := make([]mailer.Child, len(root.Children))
		for _, child := range root.Children {
			p, err := reconstructMailBodyFromDbObject(child)
			if err != nil {
				return nil, fmt.Errorf("failed to reconstruct child part: %v", err)
			}
			headerList := make([]mailer.Header, len(child.Headers))
			for _, h := range child.Headers {
				headerList = append(headerList, mailer.Header{Key: h.Key, Value: h.Value})
			}
			childParts = append(childParts, mailer.Child{
				ContentType: child.ContentType,
				Headers:     headerList,
				Part:        p,
			})
		}
		p = mailer.Multipart{
			Children: childParts,
		}
	} else if hasText {
		p = mailer.Text{Content: root.Text}
	} else if hasData {
		p = mailer.Binary{Content: root.Data}
	}

	return p, nil
}

func MailToDBEntity(m *mailer.Mail) (*sql.Mail, error) {
	toAddressList := func(addresses []mail.Address) []string {
		var outList []string
		for _, address := range addresses {
			outList = append(outList, address.String())
		}
		return outList
	}

	from := m.From.String()
	headers := make([]headerV1, 0, len(m.Headers))
	for _, h := range m.Headers {
		headers = append(headers, headerV1{Key: h.Key, Value: h.Value})
	}
	headersEnc, err := json.Marshal(headers)
	if err != nil {
		return nil, fmt.Errorf("failed to encode extra headers field: %v", err)
	}

	body := bodyV1{
		V: 1,
		Root: partV1{
			// Empty for root!
			Headers:     nil,
			ContentType: m.ContentType,
		},
	}

	if err = buildMailBodyDbObject(m.Body, &body.Root); err != nil {
		return nil, fmt.Errorf("failed to build body tree: %v", err)
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to encode body field: %v", err)
	}

	return &sql.Mail{
		Subject:      &m.Subject,
		FromAddress:  &from,
		ReplyTo:      toAddressList(m.ReplyTo),
		ToAddresses:  toAddressList(m.To),
		CcAddresses:  toAddressList(m.Cc),
		BccAddresses: toAddressList(m.Bcc),
		ExtraHeaders: headersEnc,
		MessageID:    m.MessageID,
		Body:         bodyBytes,
	}, nil
}

func DBEntityToMail(row sql.PopMailForProcessingRow) (*mailer.Mail, error) {
	fromAddressList := func(addresses []string) ([]mail.Address, error) {
		var outList []mail.Address
		for _, address := range addresses {
			parsedAddress, err := mail.ParseAddress(address)
			if err != nil {
				return nil, fmt.Errorf("failed to convert address: %s - %v", address, err)
			}
			if parsedAddress == nil || parsedAddress.Address == "" {
				return nil, fmt.Errorf("invalid address: %s - parsed: %v", address, parsedAddress)
			}
			outList = append(outList, *parsedAddress)
		}
		return outList, nil
	}

	var errorList []error

	parsedFrom, err := mail.ParseAddress(*row.FromAddress)
	if err != nil && parsedFrom == nil || parsedFrom.Address == "" {
		err = fmt.Errorf("invalid from address: %s - parsed: %v", *row.FromAddress, parsedFrom)
	}
	errorList = append(errorList, err)
	parsedReplyTo, err := fromAddressList(row.ReplyTo)
	errorList = append(errorList, err)
	parsedTo, err := fromAddressList(row.ToAddresses)

	errorList = append(errorList, err)
	parsedCc, err := fromAddressList(row.CcAddresses)
	errorList = append(errorList, err)
	parsedBcc, err := fromAddressList(row.BccAddresses)
	errorList = append(errorList, err)

	var parsedExtraHeaders []headerV1
	err = json.Unmarshal(row.ExtraHeaders, &parsedExtraHeaders)
	errorList = append(errorList, err)
	var mailerHeaders []mailer.Header
	for _, h := range parsedExtraHeaders {
		mailerHeaders = append(mailerHeaders, mailer.Header{Key: h.Key, Value: h.Value})
	}

	err = errors.Join(errorList...)
	if err != nil {
		return nil, fmt.Errorf("failed to convert DB row to mail object: %v", err)
	}

	var root bodyV1
	err = json.Unmarshal(row.Body, &root)
	if err != nil {
		return nil, fmt.Errorf("failed to decode body field: %v", err)
	}
	bodyPart, err := reconstructMailBodyFromDbObject(root.Root)
	if err != nil {
		return nil, fmt.Errorf("failed to reconstruct body from DB row: %v", err)
	}

	return &mailer.Mail{
		From:        *parsedFrom,
		ReplyTo:     parsedReplyTo,
		To:          parsedTo,
		Cc:          parsedCc,
		Bcc:         parsedBcc,
		Headers:     mailerHeaders,
		Subject:     *row.Subject,
		MessageID:   row.MessageID,
		ContentType: root.Root.ContentType,
		Body:        bodyPart,
	}, nil
}

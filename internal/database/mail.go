package database

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"

	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/generated/sql"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/pkg/mailer"
)

func MailToDBEntity(m *mailer.Mail) (*sql.Mail, error) {
	toAddressList := func(addresses []*mail.Address) []string {
		var outList []string
		for _, address := range addresses {
			outList = append(outList, address.String())
		}
		return outList
	}

	from := m.From.String()
	extraHeadersEnc, err := json.Marshal(m.ExtraHeaders)
	if err != nil {
		return nil, fmt.Errorf("failed to encode extra headers field: %v", err)
	}

	return &sql.Mail{
		Subject:      &m.Subject,
		FromAddress:  &from,
		ReplyTo:      toAddressList(m.ReplyTo),
		ToAddresses:  toAddressList(m.To),
		CcAddresses:  toAddressList(m.Cc),
		BccAddresses: toAddressList(m.Bcc),
		ExtraHeaders: extraHeadersEnc,
		Body:         &m.Body,
		MessageID:    m.MessageID,
	}, nil
}

func DBEntityToMail(row sql.PopMailForProcessingRow) (*mailer.Mail, error) {
	fromAddressList := func(addresses []string) ([]*mail.Address, error) {
		var outList []*mail.Address
		for _, address := range addresses {
			parsedAddress, err := mail.ParseAddress(address)
			if err != nil {
				return nil, fmt.Errorf("failed to convert address: %s - %v", address, err)
			}
			outList = append(outList, parsedAddress)
		}
		return outList, nil
	}

	var errorList []error

	parsedFrom, err := mail.ParseAddress(*row.FromAddress)
	errorList = append(errorList, err)
	parsedReplyTo, err := fromAddressList(row.ReplyTo)
	errorList = append(errorList, err)
	parsedTo, err := fromAddressList(row.ToAddresses)

	errorList = append(errorList, err)
	parsedCc, err := fromAddressList(row.CcAddresses)
	errorList = append(errorList, err)
	parsedBcc, err := fromAddressList(row.BccAddresses)
	errorList = append(errorList, err)

	var parsedExtraHeaders map[string][]string
	err = json.Unmarshal(row.ExtraHeaders, &parsedExtraHeaders)
	errorList = append(errorList, err)

	err = errors.Join(errorList...)
	if err != nil {
		return nil, fmt.Errorf("failed to convert DB row to mail object: %v", err)
	}

	return &mailer.Mail{
		From:         parsedFrom,
		ReplyTo:      parsedReplyTo,
		To:           parsedTo,
		Cc:           parsedCc,
		Bcc:          parsedBcc,
		ExtraHeaders: parsedExtraHeaders,
		Subject:      *row.Subject,
		Body:         *row.Body,
		MessageID:    row.MessageID,
	}, nil
}

package mailer

import (
	"context"
	"fmt"
	"net/mail"
	"strings"
	"time"
)

type MailSender interface {
	GetSender(address mail.Address) mail.Address
	MessageIDSuffix() string
	TransmitMail(ctx context.Context, m *Mail) error
}

type Mail struct {
	From         *mail.Address
	ReplyTo      []*mail.Address
	To           []*mail.Address
	Cc           []*mail.Address
	Bcc          []*mail.Address
	ExtraHeaders map[string][]string
	Subject      string
	Body         string
	MessageID    string
}

func (m *Mail) GetMessageContent() string {
	var messageBuilder strings.Builder
	fmt.Fprintf(&messageBuilder, "Date: %s\n", time.Now().Format(time.RFC822Z))
	fmt.Fprintf(&messageBuilder, "From: %s\n", m.From.String())
	fmt.Fprintf(&messageBuilder, "Message-ID: <%s>\n", m.MessageID)
	fmt.Fprintf(&messageBuilder, "Subject: %s\n", m.Subject)

	if len(m.To) > 0 {
		var toAddresses []string
		for _, toAddr := range m.To {
			toAddresses = append(toAddresses, toAddr.String())
		}
		fmt.Fprintf(&messageBuilder, "To: %s\n", strings.Join(toAddresses, "\n ,"))
	}

	if len(m.Cc) > 0 {
		var ccAddresses []string
		for _, ccAddr := range m.Cc {
			ccAddresses = append(ccAddresses, ccAddr.String())
		}
		fmt.Fprintf(&messageBuilder, "Cc: %s\n", strings.Join(ccAddresses, "\n ,"))
	}

	if len(m.ReplyTo) > 0 {
		var replyToAddresses []string
		for _, replyToAddr := range m.ReplyTo {
			replyToAddresses = append(replyToAddresses, replyToAddr.String())
		}
		fmt.Fprintf(&messageBuilder, "Reply-To: %s\n", strings.Join(replyToAddresses, "\n ,"))
	}

	messageBuilder.WriteString("\n")
	messageBuilder.WriteString(m.Body)

	messageContent := messageBuilder.String()
	messageContent = strings.ReplaceAll(messageContent, "\r\n", "\n")
	messageContent = strings.ReplaceAll(messageContent, "\n", "\r\n")

	return messageContent
}

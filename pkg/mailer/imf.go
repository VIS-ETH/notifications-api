package mailer

import (
	"fmt"
	"net/textproto"
	"strings"
	"time"
)

func (m *Mail) GetInternetMessageFormat(has8BitMimeSupport bool) (string, error) {
	if err := m.ValidateMail(); err != nil {
		return "", fmt.Errorf("mail validation failed: %v", err)
	}

	var messageBuilder strings.Builder
	fmt.Fprintf(&messageBuilder, "From: %s\n", m.From.String())

	if len(m.To) > 0 {
		fmt.Fprintf(&messageBuilder, "To: %s\n", joinAddresses(m.To))
	}

	if len(m.Cc) > 0 {
		fmt.Fprintf(&messageBuilder, "Cc: %s\n", joinAddresses(m.Cc))
	}

	if len(m.ReplyTo) > 0 {
		fmt.Fprintf(&messageBuilder, "Reply-To: %s\n", joinAddresses(m.ReplyTo))
	}

	fmt.Fprintf(&messageBuilder, "Date: %s\n", m.Date.Format(time.RFC1123Z))
	fmt.Fprintf(&messageBuilder, "Message-Id: <%s>\n", m.MessageID)
	fmt.Fprintf(&messageBuilder, "Subject: %s\n", encodedHeaderVal(m.Subject))

	hasMimeVersion := false
	for _, values := range m.Headers {
		/// values.Key was validated earlier, no control chars contained.
		canonicalHeaderKey := textproto.CanonicalMIMEHeaderKey(values.Key)
		if canonicalHeaderKey == "Mime-Version" {
			hasMimeVersion = true
		}
		// Swallowing empty values is legal
		if values.Value == "" {
			continue
		}
		fmt.Fprintf(&messageBuilder, "%s: %s\n", canonicalHeaderKey, encodedHeaderVal(values.Value))
	}
	if !hasMimeVersion && m.ContentType != "" {
		messageBuilder.WriteString("Mime-Version: 1.0\n")
	}

	encodedPart := m.Body.encode(m.ContentType, has8BitMimeSupport)
	if m.ContentType != "" {
		fmt.Fprintf(&messageBuilder, "Content-Type: %s\n", encodedHeaderVal(encodedPart.contentType))
	}
	fmt.Fprintf(&messageBuilder, "Content-Transfer-Encoding: %s\n", encodedHeaderVal(encodedPart.contentTransferEncoding))

	messageBuilder.WriteString("\n")
	messageBuilder.WriteString(encodedPart.content)

	messageContent := messageBuilder.String()
	messageContent = strings.ReplaceAll(messageContent, "\r\n", "\n")
	messageContent = strings.ReplaceAll(messageContent, "\r", "\n")
	messageContent = strings.ReplaceAll(messageContent, "\n", "\r\n")

	return messageContent, nil
}

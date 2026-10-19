package mailer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"mime"
	"mime/multipart"
	"net/mail"
	"net/textproto"
	"strings"
	"time"
	"unicode"
)

var reservedHeaders = map[string]struct{}{
	"From":         {},
	"To":           {},
	"Cc":           {},
	"Bcc":          {},
	"Reply-To":     {},
	"Subject":      {},
	"Date":         {},
	"Message-Id":   {},
	"Content-Type": {},
}

//mockery:generate: true
type MailSender interface {
	GetSender(address mail.Address) mail.Address
	MessageIDSuffix() string
	TransmitMail(ctx context.Context, m *Mail) error
}

type Mail struct {
	From    mail.Address
	ReplyTo []mail.Address
	To      []mail.Address
	Cc      []mail.Address
	Bcc     []mail.Address

	Subject     string
	Date        time.Time
	MessageID   string
	ContentType string

	Headers []Header
	Body    Part
}

type Header struct{ Key, Value string }

func ValidateHeaderKey(headerKey string) error {
	for _, r := range headerKey {
		// RFC 5322 field names
		// printable ascii (0x20 - 0x7e) without space (0x20) and colon (0x3a)
		// go blame me if numbers are wrong, but wikipedia agrees.
		if r <= 0x20 || r >= 0x7f || r == ':' {
			return fmt.Errorf("invalid header key %q: invalid character %U", headerKey, r)
		}
	}
	return nil
}

func ValidateHeaderVal(headerVal string) error {
	if idx := strings.IndexFunc(headerVal, unicode.IsControl); idx != -1 {
		return fmt.Errorf("invalid header value %q: invalid control character %U at index %d", headerVal, rune(headerVal[idx]), idx)
	}
	return nil
}

func (m *Mail) ValidateMail() error {
	var validationErrors []error

	if err := ValidateHeaderVal(m.Subject); err != nil {
		validationErrors = append(validationErrors, fmt.Errorf("invalid subject %q: %v", m.Subject, err))
	}

	if m.ContentType != "" {
		parsedMediaType, params, err := mime.ParseMediaType(m.ContentType)
		if err != nil {
			validationErrors = append(validationErrors, fmt.Errorf("invalid content type %q: %v", m.ContentType, err))
		}

		if _, ok := m.Body.(Multipart); ok {
			if !strings.HasPrefix(parsedMediaType, "multipart/") {
				validationErrors = append(validationErrors, fmt.Errorf("invalid content type %q for multipart body: should match multipart/*", m.ContentType))
			}

			if boundary, hasBoundary := params["boundary"]; hasBoundary {
				var buf bytes.Buffer
				w := multipart.NewWriter(&buf)
				defer func() { _ = w.Close() }()
				if err := w.SetBoundary(boundary); err != nil {
					validationErrors = append(validationErrors, fmt.Errorf("invalid boundary %q in content type %q: %v", boundary, m.ContentType, err))
				}
			}
		}
	}

	for _, h := range m.Headers {
		if err := ValidateHeaderKey(h.Key); err != nil {
			validationErrors = append(validationErrors, fmt.Errorf("invalid header key %q: %v", h.Key, err))
		}
		if err := ValidateHeaderVal(h.Value); err != nil {
			validationErrors = append(validationErrors, fmt.Errorf("invalid header value for key %q: %v", h.Key, err))
		}

		canonicalHeaderKey := textproto.CanonicalMIMEHeaderKey(h.Key)
		if _, isReserved := reservedHeaders[canonicalHeaderKey]; isReserved {
			validationErrors = append(validationErrors, fmt.Errorf("invalid header key %q: reserved header key", h.Key))
		}
	}

	if err := m.Body.validate(m.ContentType); err != nil {
		validationErrors = append(validationErrors, fmt.Errorf("invalid body: %v", err))
	}

	return errors.Join(validationErrors...)
}

func IsHeaderKeyReserved(headerKey string) bool {
	_, isReserved := reservedHeaders[textproto.CanonicalMIMEHeaderKey(headerKey)]
	return isReserved
}

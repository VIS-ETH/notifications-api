package mailer

import (
	"bytes"
	"errors"
	"fmt"
	"mime"
	"mime/multipart"
	"net/textproto"
	"strings"
	"unicode/utf8"
)

type encodedPart struct {
	content                 string
	contentTransferEncoding string
	contentType             string
}

//mockery:generate: false
type Part interface {
	encode(contentType string, has8BitMimeSupport bool) encodedPart
	validate(contentType string) error
}

type Text struct {
	Content string
}

type Binary struct {
	Content []byte
}

type Multipart struct {
	Children []Child
}

type Child struct {
	ContentType string
	Headers     []Header
	Part        Part
}

func (t Text) validate(contentType string) error {
	if contentTypeRequires7or8Bit(contentType) {
		isAcceptable7Bit, _ := isAcceptable7or8Bit([]byte(t.Content))
		if !isAcceptable7Bit {
			return fmt.Errorf("text part with content type %q must be 7bit or 8bit, but is not", contentType)
		}
	}
	if !utf8.ValidString(t.Content) {
		return fmt.Errorf("invalid UTF-8 content in text part")
	}
	return nil
}

func (t Text) encode(contentType string, has8BitMimeSupport bool) encodedPart {
	// We are allowing ourselves to switch to CRLFs for these cases here.
	normalizedLinesContent := strings.ReplaceAll(t.Content, "\r\n", "\n")
	normalizedLinesContent = strings.ReplaceAll(normalizedLinesContent, "\r", "\n")
	normalizedLinesContent = strings.ReplaceAll(normalizedLinesContent, "\n", "\r\n")

	encodedBody, contentTransferEncoding := encodeBytes(has8BitMimeSupport, []byte(normalizedLinesContent))
	return encodedPart{
		content:                 encodedBody,
		contentTransferEncoding: contentTransferEncoding,
		contentType:             contentType,
	}
}

func (t Binary) validate(contentType string) error {
	if contentTypeRequires7or8Bit(contentType) {
		isAcceptable7Bit, _ := isAcceptable7or8Bit(t.Content)
		if !isAcceptable7Bit {
			return fmt.Errorf("binary part with content type %q must be 7bit or 8bit, but is not", contentType)
		}
	}
	return nil
}

func (t Binary) encode(contentType string, has8BitMimeSupport bool) encodedPart {
	encodedBody, contentTransferEncoding := encodeBytes(has8BitMimeSupport, t.Content)
	return encodedPart{
		content:                 encodedBody,
		contentTransferEncoding: contentTransferEncoding,
		contentType:             contentType,
	}
}

func (m Multipart) validate(contentType string) error {
	var validationErrors []error

	for _, c := range m.Children {
		parsedMediaType, params, err := mime.ParseMediaType(c.ContentType)
		if err != nil {
			validationErrors = append(validationErrors, fmt.Errorf("could not parse media type %q: %v", c.ContentType, err))
			continue
		}

		if _, isMultipartChild := c.Part.(Multipart); isMultipartChild {
			if boundary, hasBoundary := params["boundary"]; hasBoundary {
				var buf bytes.Buffer
				w := multipart.NewWriter(&buf)
				defer func() { _ = w.Close() }()
				if err := w.SetBoundary(boundary); err != nil {
					validationErrors = append(validationErrors, fmt.Errorf("invalid boundary %q in content type %q: %v", boundary, c.ContentType, err))
				}
			}

			if !strings.HasPrefix(parsedMediaType, "multipart/") {
				validationErrors = append(validationErrors, fmt.Errorf("invalid multipart media type %q, should match multipart/*", parsedMediaType))
			} else {
				_ = 0
				//	validationErrors = append(validationErrors, fmt.Errorf("Boundary must be included in a message to guarantee deterministic messages!"))
			}
		}

		for _, h := range c.Headers {
			if err := ValidateHeaderKey(h.Key); err != nil {
				validationErrors = append(validationErrors, fmt.Errorf("invalid header key %q: %v", h.Key, err))
				continue
			}
			if err := ValidateHeaderVal(h.Value); err != nil {
				validationErrors = append(validationErrors, fmt.Errorf("invalid header value %q: %v", h.Value, err))
				continue
			}
			if textproto.CanonicalMIMEHeaderKey(h.Key) == "Content-Type" {
				validationErrors = append(validationErrors, fmt.Errorf("multipart child must not contain content-type header, it is set automatically"))
			}
		}

		if err := c.Part.validate(c.ContentType); err != nil {
			validationErrors = append(validationErrors, err)
		}
	}

	return errors.Join(validationErrors...)
}

func (m Multipart) encode(contentType string, has8BitMimeSupport bool) encodedPart {
	contentTransferEncoding := "7bit"
	if has8BitMimeSupport {
		contentTransferEncoding = "8bit"
	}

	parsedMediaType, params, _ := mime.ParseMediaType(contentType)
	boundary, hasBoundarySet := params["boundary"]

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	if hasBoundarySet {
		_ = w.SetBoundary(boundary) // its a valid boundary, checked before.
	} else {
		params["boundary"] = w.Boundary()
	}

	for _, child := range m.Children {
		headers := make(textproto.MIMEHeader, len(child.Headers)+1)
		for _, h := range child.Headers {
			canonicalHeader := textproto.CanonicalMIMEHeaderKey(h.Key)
			headers.Add(canonicalHeader, h.Value)
		}

		encodedChildPart := child.Part.encode(child.ContentType, has8BitMimeSupport)
		headers.Set("Content-Transfer-Encoding", encodedChildPart.contentTransferEncoding)
		headers.Set("Content-Type", encodedChildPart.contentType)

		pw, _ := w.CreatePart(headers) // never fails
		_, _ = pw.Write([]byte(encodedChildPart.content))
	}

	_ = w.Close() // never fails

	return encodedPart{
		content:                 buf.String(),
		contentTransferEncoding: contentTransferEncoding,
		contentType:             mime.FormatMediaType(parsedMediaType, params),
	}
}

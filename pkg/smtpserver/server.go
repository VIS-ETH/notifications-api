package smtpserver

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
	"github.com/sirupsen/logrus"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/pkg/mailer"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/text/encoding/htmlindex"
)

var smtpServerTracer = otel.Tracer("smtpserver/receiver")

type authHandler func(ctx context.Context, username, password string) (context.Context, error)

type messageHandler func(ctx context.Context, mail *mailer.Mail) error

type Backend struct {
	handleMessage messageHandler
	handleAuth    authHandler

	requireAuth bool
}

/*
netcat -C localhost 1025
EHLO localhost
MAIL FROM:<root@nsa.gov>
RCPT TO:<root@gchq.gov.uk>
DATA
From: <root@nsa.gov>
Subject: Test

Hey <3
*/
func NewBackend(
	handleMessage messageHandler,
	options ...BackendOption,
) *Backend {
	b := &Backend{handleMessage: handleMessage}
	for _, o := range options {
		o(b)
	}
	return b
}

type BackendOption func(*Backend)

func WithAuthHandler(handleAuth authHandler) BackendOption {
	return func(b *Backend) {
		b.requireAuth = true
		b.handleAuth = handleAuth
	}
}

func (bkd *Backend) NewSession(c *smtp.Conn) (smtp.Session, error) {
	logger := logrus.WithFields(logrus.Fields{
		"component": "smtpserver",
	})

	connCtx, connSpan := smtpServerTracer.Start(
		context.Background(), "smtp.Connection",
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithAttributes(
			semconv.NetworkProtocolName("smtp"),
			semconv.NetworkPeerAddress(c.Conn().RemoteAddr().String()),
		),
	)

	s := &session{
		backend:  bkd,
		logger:   logger,
		rootCtx:  connCtx,
		connSpan: connSpan,
	}
	s.Reset()
	return s, nil
}

type session struct {
	backend *Backend
	logger  *logrus.Entry

	from       string
	recipients []string
	data       []byte

	connSpan        trace.Span
	ctx             context.Context
	rootCtx         context.Context
	isAuthenticated bool
	hadError        bool
}

func (s *session) AuthMechanisms() []string {
	// Technically, we could support OAuth... it's ugly and not well supported. so no.
	return []string{sasl.Plain}
}

func (s *session) Auth(mech string) (sasl.Server, error) {
	switch mech {
	case sasl.Plain:
		return sasl.NewPlainServer(func(identity, username, password string) error {
			authedCtx, err := s.backend.handleAuth(s.ctx, username, password)
			if err != nil {
				return fmt.Errorf("authentication failed: %v", err)
			}
			s.ctx = authedCtx
			s.isAuthenticated = true
			return nil
		}), nil
		//case sasl.OAuthBearer:
		//return sasl.NewOAuthBearerServer(func(opts sasl.OAuthBearerOptions) *sasl.OAuthBearerError {
		//	// Verify signature, load & check roles
		//	opts.Token
		//}), nil
	}
	return nil, fmt.Errorf("no mechanism %s supported", mech)
}

func (s *session) AuthPlain(username, password string) error {
	if !s.backend.requireAuth {
		return fmt.Errorf("authentication not required, but client attempted to authenticate")
	}
	authedCtx, err := s.backend.handleAuth(s.ctx, username, password)
	if err != nil {
		return fmt.Errorf("authentication failed: %v", err)
	}
	s.ctx = authedCtx
	s.isAuthenticated = true
	return nil
}

func (s *session) Logout() error {
	if s.connSpan != nil {
		if s.hadError {
			s.connSpan.SetStatus(codes.Error, "session had at least one failed message")
		} else {
			s.connSpan.SetStatus(codes.Ok, "session closed")
		}
		s.connSpan.End()
	}
	return nil
}

func (s *session) Reset() {
	s.from = ""
	s.recipients = nil
	s.data = nil
	s.ctx = s.rootCtx
	s.isAuthenticated = false
}

func (s *session) Mail(from string, opts *smtp.MailOptions) error {
	if s.backend.requireAuth && !s.isAuthenticated {
		return smtp.ErrAuthRequired
	}
	s.from = from
	return nil
}

func (s *session) Rcpt(to string, opts *smtp.RcptOptions) error {
	if s.backend.requireAuth && !s.isAuthenticated {
		return smtp.ErrAuthRequired
	}
	s.recipients = append(s.recipients, to)
	return nil
}

func (s *session) Data(r io.Reader) (err error) {
	if s.backend.requireAuth && !s.isAuthenticated {
		return smtp.ErrAuthRequired
	}

	ctx, span := smtpServerTracer.Start(
		s.ctx, "smtp.ReceiveMail",
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithAttributes(
			attribute.String("smtp.from", s.from),
			attribute.Int("smtp.recipient_count", len(s.recipients)),
		),
	)
	defer span.End()
	defer func() {
		if err != nil {
			s.hadError = true
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			s.logger.Warnf("Failed to receive mail: %v", err)
			return
		}
		span.SetStatus(codes.Ok, "mail received")

		so, soErr := getSMTPObservability()
		if soErr != nil {
			s.logger.Warnf("Failed to get smtp server observability! %v", soErr)
			return
		}
		(*so.counter).Add(ctx, 1)

		s.logger.Info("Received mail successfully")
	}()

	message, err := mail.ReadMessage(r)
	if err != nil {
		return fmt.Errorf("reading message from %s to [%s] failed: %v", s.from, strings.Join(s.recipients, ", "), err)
	}

	parsedMail := &mailer.Mail{}

	fromList, err := getAddressList(message, "From")
	if err != nil {
		return fmt.Errorf("parsing from header failed: %v", err)
	} else if len(fromList) != 1 {
		// allowed to be picky here, that would be stupid...
		return fmt.Errorf("expected exactly one from address, got %d... aborting", len(fromList))
	}
	parsedMail.From = fromList[0]
	if fromList[0].Address != s.from {
		return fmt.Errorf("from header address %s does not match SMTP from %s", fromList[0].Address, s.from)
	}

	replyTo, err := getAddressList(message, "Reply-To")
	if err != nil && !errors.Is(err, mail.ErrHeaderNotPresent) {
		return fmt.Errorf("parsing reply-to header failed: %v", err)
	}
	parsedMail.ReplyTo = replyTo
	to, _ := getAddressList(message, "To")
	if errors.Is(err, mail.ErrHeaderNotPresent) {
		goto toRecipientsOk
	}
	if err != nil {
		return fmt.Errorf("parsing to header failed: %v", err)
	}
	for _, a := range to {
		for _, rcpt := range s.recipients {
			if a.Address == rcpt {
				goto toRecipientsOk
			}
		}
	}
	return fmt.Errorf("to header addresses not found in SMTP recipients list")
toRecipientsOk:
	parsedMail.To = to
	cc, err := getAddressList(message, "Cc")
	if errors.Is(err, mail.ErrHeaderNotPresent) {
		goto ccRecipientsOk
	}
	if err != nil {
		return fmt.Errorf("parsing cc header failed: %v", err)
	}
	for _, a := range cc {
		for _, rcpt := range s.recipients {
			if a.Address == rcpt {
				goto ccRecipientsOk
			}
		}
	}
	return fmt.Errorf("cc header addresses not found in SMTP recipients list")
ccRecipientsOk:
	parsedMail.Cc = cc

	parsedMail.Subject = decodeRFC2047(message.Header.Get("Subject"))
	if messageId := message.Header.Get("Message-ID"); messageId != "" {
		if messageId[0] != '<' && messageId[len(messageId)-1] != '>' {
			return fmt.Errorf("invalid message id header %q", messageId)
		}
		parsedMail.MessageID = messageId[1 : len(messageId)-1]
		span.SetAttributes(attribute.String("smtp.message_id", parsedMail.MessageID))
	}

	extraHeaders := make(map[string][]string)
	for key, values := range message.Header {
		lower := strings.ToLower(key)
		// skip headers we already explicitly extracted
		switch lower {
		case "from", "reply-to", "to", "cc", "subject", "message-id":
			continue
		case "content-type", "date":
			// handled explicitly below, too
			continue
		case "content-transfer-encoding":
			// as things might be re-encoded (7bit support), needs to be decoded first.
			continue
		case "mime-version":
			// mime-version header is always set to 1.0 by mailer, ignore unless explicitly set!
			for _, v := range values {
				if v != "1.0" {
					goto handleHeader
				}
			}
			continue
		}
	handleHeader:
		var decodedValues []string
		for _, v := range values {
			decodedValues = append(decodedValues, decodeRFC2047(v))
		}
		extraHeaders[key] = decodedValues
	}

	seen := make(map[string]bool)
	for _, a := range to {
		seen[strings.ToLower(a.Address)] = true
	}
	for _, a := range cc {
		seen[strings.ToLower(a.Address)] = true
	}
	var bcc []mail.Address
	for _, rcpt := range s.recipients {
		if !seen[strings.ToLower(rcpt)] {
			bcc = append(bcc, mail.Address{Address: rcpt})
		}
	}
	parsedMail.Bcc = bcc

	parsedMail.ContentType = message.Header.Get("Content-Type")
	if dateHeader := message.Header.Get("Date"); dateHeader != "" {
		parsedMail.Date, err = time.Parse(time.RFC1123Z, dateHeader)
		if err != nil {
			return fmt.Errorf("parsing date header failed: %v", err)
		}
	}

	if message.Header.Get("Mime-Version") != "" {
		body, err := parsePart(textproto.MIMEHeader(message.Header), message.Body)
		if err != nil {
			return fmt.Errorf("parsing message body from %s to [%s] failed: %v", s.from, strings.Join(s.recipients, ", "), err)
		}
		parsedMail.Body = body
	} else {
		b, err := io.ReadAll(message.Body)
		if err != nil {
			return fmt.Errorf("reading message body from %s to [%s] failed: %v", s.from, strings.Join(s.recipients, ", "), err)
		}
		parsedMail.Body = mailer.Binary{Content: b}
	}

	if err = parsedMail.ValidateMail(); err != nil {
		return fmt.Errorf("failed to validate parsed mail: %v", err)
	}

	err = s.backend.handleMessage(ctx, parsedMail)
	if err != nil {
		return fmt.Errorf("handling message from %s to [%s] failed: %v", s.from, strings.Join(s.recipients, ", "), err)
	}

	return nil
}

// In general, content-transfer-encoding is easy enough to decode/encode.
// However, turns out that some really smart people thought it necessary to add
// "composite" types, and created cases where the usual content-transfer-encoding
// does _not_ apply throughout the entire message body. Hence, we need to parse
// these things and encode/decode accordingly.
//
// Note: issue would be that, if a mailserver only supports 7bit, we have to re-encode
// the message, as our own server does 8bit. Also, aesthetically pleasing end-result.
//
// Uhm, according to RFC 2045, only such composite types are multipart/* and message/*,
// where multipart/* is handled recursively.
func parsePart(header textproto.MIMEHeader, body io.Reader) (mailer.Part, error) {
	mediaType, params, err := mime.ParseMediaType(header.Get("Content-Type"))
	if err != nil {
		// No or malformed Content-Type: Mime RFC default... can be used/assumed if content-type is invalid
		mediaType, params = "text/plain", map[string]string{"charset": "us-ascii"}
	}

	decoded := decodeTransferEncoding(header.Get("Content-Transfer-Encoding"), body)

	if strings.HasPrefix(mediaType, "multipart/") {
		if cte := strings.ToLower(header.Get("Content-Transfer-Encoding")); cte != "" && cte != "7bit" && cte != "8bit" && cte != "binary" {
			return nil, fmt.Errorf("media type %s is multipart. only 8bit, 7bit, binary allowed on multipart messages, but was %s", mediaType, cte)
		}

		boundary := params["boundary"]
		if boundary == "" {
			return nil, fmt.Errorf("%s part has no boundary parameter, malformed", mediaType)
		}
		mr := multipart.NewReader(decoded, boundary)
		var children []mailer.Child
		for {
			p, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("reading %s part: %w", mediaType, err)
			}
			child, err := parseChild(p.Header, p)
			if err != nil {
				return nil, fmt.Errorf("parsing %s child part: %w", mediaType, err)
			}
			children = append(children, child)
		}
		return mailer.Multipart{Children: children}, nil
	}

	raw, err := io.ReadAll(decoded)
	if err != nil {
		return nil, fmt.Errorf("reading %s part: %w", mediaType, err)
	}

	// try and detect proper, nice text.
	if strings.HasPrefix(mediaType, "text/") {
		if text := decodeText(raw, params["charset"]); utf8.ValidString(text) {
			return mailer.Text{Content: strings.ReplaceAll(text, "\r\n", "\n")}, nil
		}
	}
	return mailer.Binary{Content: raw}, nil
}

// parseChild builds a mailer.Child for one sub-part of a multipart entity:
// its decoded Part plus its non-structural headers (Content-Type and
// Content-Transfer-Encoding are excluded, since Multipart.encode sets both
// automatically and Multipart.validate rejects an explicit Content-Type
// among Headers).
func parseChild(header textproto.MIMEHeader, body io.Reader) (mailer.Child, error) {
	part, err := parsePart(header, body)
	if err != nil {
		return mailer.Child{}, err
	}

	var headers []mailer.Header
	for key, values := range header {
		switch textproto.CanonicalMIMEHeaderKey(key) {
		case "Content-Type", "Content-Transfer-Encoding":
			continue
		}
		for _, v := range values {
			headers = append(headers, mailer.Header{Key: key, Value: decodeRFC2047(v)})
		}
	}

	c := mailer.Child{
		ContentType: header.Get("Content-Type"),
		Part:        part,
	}
	if len(headers) > 0 {
		c.Headers = headers
	}
	return c, nil
}

func decodeTransferEncoding(cte string, body io.Reader) io.Reader {
	switch normalizeCTE(cte) {
	case "quoted-printable":
		return quotedprintable.NewReader(body)
	case "base64":
		return base64.NewDecoder(base64.StdEncoding, body)
	default:
		return body
	}
}

// since we're doing mail hardcore style, might as well use some kind of
// charset that is "sinnvoll", like utf-8, instead of microsoft-09150912830981240
// or whatever other 500 charsets people come up with.
func decodeText(raw []byte, charset string) string {
	charset = normalizeCTE(charset)
	if charset == "" || charset == "us-ascii" || charset == "ascii" || charset == "utf-8" {
		return string(raw)
	}
	enc, err := htmlindex.Get(charset)
	if err != nil {
		return string(raw)
	}
	decoded, err := enc.NewDecoder().Bytes(raw)
	if err != nil {
		return string(raw)
	}
	return string(decoded)
}

func getAddressList(m *mail.Message, headername string) ([]mail.Address, error) {
	if m == nil {
		return nil, nil
	}

	l, err := m.Header.AddressList(headername)
	if err != nil {
		return nil, fmt.Errorf("failed to parse %s header: %w", headername, err)
	}

	var result []mail.Address
	for _, addr := range l {
		if addr == nil || addr.Address == "" {
			return nil, fmt.Errorf("invalid %s header: empty address. parsed: %v", headername, addr)
		}
		result = append(result, *addr)
	}

	return result, nil
}

func decodeRFC2047(s string) string {
	dec := new(mime.WordDecoder)
	d, err := dec.DecodeHeader(s)
	if err != nil {
		return s // fallback to raw
	}
	return d
}

// Of course there exists mails that use the fact that cte is case-insensitive
// AND CAN HAVE WHITESPACES sprinkled in... :) Good RFCs
func normalizeCTE(cte string) string {
	return strings.ToLower(strings.TrimSpace(cte))
}

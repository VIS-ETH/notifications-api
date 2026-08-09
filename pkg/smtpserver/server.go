package smtpserver

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"mime"
	"net/mail"
	"strings"
	"time"

	"github.com/emersion/go-smtp"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/pkg/mailer"
)

type messageHandler func(ctx context.Context, mail *mailer.Mail) error

type Backend struct {
	handleMessage messageHandler
}

func NewBackend(handleMessage messageHandler) *Backend {
	return &Backend{handleMessage: handleMessage}
}

func (bkd *Backend) NewSession(c *smtp.Conn) (smtp.Session, error) {
	return &session{
		backend: bkd,
	}, nil
}

type session struct {
	// auth    bool
	backend *Backend

	from       string
	recipients []string
	data       []byte
}

/*
func (s *session) AuthMechanisms() []string {
	return []string{sasl.Plain, sasl.OAuthBearer}
}

func (s *session) Auth(mech string) (sasl.Server, error) {
	switch mech {
	case sasl.Plain:
		return sasl.NewPlainServer(func(identity, username, password string) error {
			if username != "username" || password != "password" {
				return errors.New("invalid username or password")
			}
			s.auth = true
			return nil
		}), nil
	case sasl.OAuthBearer:
		//return sasl.NewOAuthBearerServer(func(opts sasl.OAuthBearerOptions) *sasl.OAuthBearerError {
		//	// Verify signature, load & check roles
		//	opts.Token
		//}), nil
	}
	return nil, fmt.Errorf("tedstg")
}

func (s *session) Logout() error {
	return nil
}
*/

func (s *session) AuthPlain(username, password string) error {
	return nil
}

func (s *session) Logout() error {
	return nil
}

func (s *session) Reset() {
	s.from = ""
	s.recipients = nil
	s.data = nil
}

func (s *session) Mail(from string, opts *smtp.MailOptions) error {
	/*
		if !s.auth {
			return smtp.ErrAuthRequired
		}
	*/
	s.from = from
	return nil
}

func (s *session) Rcpt(to string, opts *smtp.RcptOptions) error {
	/*
		if !s.auth {
			return smtp.ErrAuthRequired
		}
	*/
	s.recipients = append(s.recipients, to)
	return nil
}

func (s *session) Data(r io.Reader) error {
	/*
		if !s.auth {
			return smtp.ErrAuthRequired
		}
	*/
	message, err := mail.ReadMessage(r)
	if err != nil {
		return fmt.Errorf("reading message from %s to [%s] failed: %v", s.from, strings.Join(s.recipients, ", "), err)
	}

	parsedMail := &mailer.Mail{}

	fromList, err := message.Header.AddressList("From")
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

	replyTo, err := message.Header.AddressList("Reply-To")
	if err != nil && err != mail.ErrHeaderNotPresent {
		return fmt.Errorf("parsing reply-to header failed: %v", err)
	}
	parsedMail.ReplyTo = replyTo
	to, _ := message.Header.AddressList("To")
	if err == mail.ErrHeaderNotPresent {
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
	cc, err := message.Header.AddressList("Cc")
	if err == mail.ErrHeaderNotPresent {
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
	parsedMail.MessageID = message.Header.Get("Message-ID")

	extraHeaders := make(map[string][]string)
	for key, values := range message.Header {
		lower := strings.ToLower(key)
		// skip headers we already explicitly extracted
		switch lower {
		case "from", "reply-to", "to", "cc", "subject", "message-id":
			continue
		}
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
	var bcc []*mail.Address
	for _, rcpt := range s.recipients {
		if !seen[strings.ToLower(rcpt)] {
			bcc = append(bcc, &mail.Address{Address: rcpt})
		}
	}
	parsedMail.Bcc = bcc

	err = s.backend.handleMessage(context.TODO(), parsedMail)
	if err != nil {
		return fmt.Errorf("handling message from %s to [%s] failed: %v", s.from, strings.Join(s.recipients, ", "), err)
	}

	return nil
}

func decodeRFC2047(s string) string {
	dec := new(mime.WordDecoder)
	d, err := dec.DecodeHeader(s)
	if err != nil {
		return s // fallback to raw
	}
	return d
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
func ExampleServer() {
	be := &Backend{}

	s := smtp.NewServer(be)

	s.Addr = "localhost:1025"
	s.Domain = "localhost"
	s.WriteTimeout = 10 * time.Second
	s.ReadTimeout = 10 * time.Second
	s.MaxMessageBytes = 1024 * 1024
	s.MaxRecipients = 50
	s.AllowInsecureAuth = true

	s.EnableREQUIRETLS = true

	c, err := tls.LoadX509KeyPair("", "")
	if err != nil {
		panic("A")
	}
	s.TLSConfig = &tls.Config{
		Certificates: []tls.Certificate{c},
	}

	log.Println("Starting server at", s.Addr)
	if err := s.ListenAndServeTLS(); err != nil {
		log.Fatal(err)
	}
}

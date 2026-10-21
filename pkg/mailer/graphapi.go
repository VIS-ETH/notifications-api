package mailer

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/mail"
	"strings"

	"github.com/sirupsen/logrus"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal/auth"
)

const (
	TokenScope    = "https://graph.microsoft.com/.default"
	TokenEndpoint = "https://login.microsoftonline.com/%s/oauth2/v2.0/token"
	MailEndpoint  = "https://graph.microsoft.com/v1.0//users/%s/sendMail"
)

type GraphAPIMailSender struct {
	senderEmail     string
	messageIDSuffix string

	tp     *auth.OidcTokenProvider
	logger *logrus.Entry
}

func NewGraphAPIMailSender(tenantID, clientID, clientSecret, senderEmail, messageIDSuffix string) *GraphAPIMailSender {
	mailLogger := logrus.WithFields(logrus.Fields{
		"component": "graphapi-mail",
	})

	tp := auth.NewOidcTokenProvider(
		fmt.Sprintf(TokenEndpoint, tenantID),
		clientID, clientSecret,
		auth.WithScope(TokenScope),
	)

	return &GraphAPIMailSender{
		tp:              tp,
		senderEmail:     senderEmail,
		messageIDSuffix: messageIDSuffix,
		logger:          mailLogger,
	}
}

func (g *GraphAPIMailSender) TransmitMail(ctx context.Context, m *Mail) error {
	imfMail, err := m.GetInternetMessageFormat(true)
	if err != nil {
		return fmt.Errorf("failed to encode mail into internet message format: %v", err)
	}
	base64EncodedMail := base64.StdEncoding.EncodeToString([]byte(imfMail))
	reader := strings.NewReader(base64EncodedMail)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf(MailEndpoint, g.senderEmail), reader)
	if err != nil {
		return fmt.Errorf("failed to build send-mail-request: %v", err)
	}
	accessToken, err := g.tp.GetAccessToken()
	if err != nil {
		return fmt.Errorf("failed to get access token for graph api: %v", err)
	}
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", *accessToken))
	req.Header.Add("Content-Type", "text/plain")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("post request to send mail failed: %v", err)
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	} else {
		g.logger.Errorf("failed to send mail via graph api, status code: %d", resp.StatusCode)
		var respBody bytes.Buffer
		defer func() { _ = resp.Body.Close() }()
		if _, err := respBody.ReadFrom(resp.Body); err != nil {
			g.logger.Errorf("failed to read response body: %v", err)
			return fmt.Errorf("server did not accept encoded mail! status code %d, failed to read response body: %v", resp.StatusCode, err)
		}
		g.logger.Errorf("response body: %s", respBody.String())
	}
	return fmt.Errorf("server did not accept encoded mail! status code %d", resp.StatusCode)
}

func (g *GraphAPIMailSender) MessageIDSuffix() string {
	return g.messageIDSuffix
}

func (g *GraphAPIMailSender) GetSender(address mail.Address) mail.Address {
	return mail.Address{Address: g.senderEmail}
}

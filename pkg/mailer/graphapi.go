package mailer

import (
	"fmt"

	"github.com/sirupsen/logrus"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal/auth"
)

const (
	TokenScope    = "https://graph.microsoft.com/.default"
	TokenEndpoint = "https://login.microsoftonline.com/%s/oauth2/v2.0/token"
	MailEndpoint  = "https://graph.microsoft.com/v1.0//users/%s/sendMail"
)

type GraphAPIMailSender struct {
	senderEmail string
	tp          *auth.OidcTokenProvider
	logger      *logrus.Entry
}

func NewGraphAPIMailSender(tenantID, clientID, clientSecret, senderEmail string) *GraphAPIMailSender {
	mailLogger := logrus.WithFields(logrus.Fields{
		"component": "graphapi-mail",
	})

	tp := auth.NewOidcTokenProvider(
		fmt.Sprintf(TokenEndpoint, tenantID),
		clientID, clientSecret,
		auth.WithScope(TokenScope),
	)

	return &GraphAPIMailSender{
		tp:          tp,
		senderEmail: senderEmail,
		logger:      mailLogger,
	}
}

/*
func (g *GraphAPIMailSender) GetSender(address mail.Address) mail.Address {
	return address
}
*/

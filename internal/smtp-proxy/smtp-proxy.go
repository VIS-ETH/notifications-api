package smtpproxy

import (
	"context"
	"fmt"

	"github.com/emersion/go-smtp"
	"github.com/sirupsen/logrus"
	pb "gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/generated/pb/sip/notifications"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal/auth"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/pkg/mailer"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/pkg/smtpserver"
	"google.golang.org/grpc/metadata"
)

type (
	GrpcAuthMode              string
	SMTPAuthMode              string
	SMTPSessionContextAuthKey string
)

const (
	GrpcAuthModeSMTPPassthrough GrpcAuthMode = "grpc-auth-smtp-passthrough"
	GrpcAuthModeOIDCInject      GrpcAuthMode = "grpc-auth-oidc-inject"
	GrpcAuthModeNone            GrpcAuthMode = "grpc-auth-unauthenticated"
)

const (
	SMTPAuthModeNone  SMTPAuthMode = "smtp-auth-none"
	SMTPAuthModePlain SMTPAuthMode = "smtp-auth-plain"
)

const SMTPSessionContextAuthKeyName SMTPSessionContextAuthKey = "smtp-server-auth-handler-context-key"

type OIDCConfig struct {
	oidcTokenEndpoint string
	oidcClientID      string
	oidcClientSecret  string
}

type SMTPProxyConfig struct {
	SMTPAuthMode         SMTPAuthMode
	SMTPEnsureSender     bool
	GrpcAuthMode         GrpcAuthMode
	OidcConfig           *OIDCConfig
	TokenProviderOptions []auth.OidcTokenProviderOption
	LoggingOnly          bool
	logger               *logrus.Entry
}

func NewOIDCConfig(tokenEndpoint, clientID, clientSecret string) *OIDCConfig {
	return &OIDCConfig{
		oidcTokenEndpoint: tokenEndpoint,
		oidcClientID:      clientID,
		oidcClientSecret:  clientSecret,
	}
}

func GetSMTPServer(config SMTPProxyConfig, client pb.MailServiceClient) (*smtp.Server, error) {
	config.logger = logrus.WithFields(logrus.Fields{
		"component": "smtp-proxy-handler",
	})

	var oidcTokenProvider *auth.OidcTokenProvider
	if config.GrpcAuthMode == GrpcAuthModeOIDCInject {
		oidcTokenProvider = auth.NewOidcTokenProvider(
			config.OidcConfig.oidcTokenEndpoint,
			config.OidcConfig.oidcClientID,
			config.OidcConfig.oidcClientSecret,
			config.TokenProviderOptions...,
		)
		if _, err := oidcTokenProvider.GetAccessToken(); err != nil {
			return nil, fmt.Errorf("failed to get initial access token: %v", err)
		}
	}

	mailHandler := func(ctx context.Context, mail *mailer.Mail) error {
		config.logger.Debugf("Received mail to send via gRPC: Subject=%s, From=%s, To=%v", mail.Subject, mail.From.Address, mail.To)

		to := make([]*pb.MailAddress, len(mail.To))
		for i, addr := range mail.To {
			to[i] = mailer.MakePbAddress(addr.Name, addr.Address)
		}
		cc := make([]*pb.MailAddress, len(mail.Cc))
		for i, addr := range mail.Cc {
			cc[i] = mailer.MakePbAddress(addr.Name, addr.Address)
		}
		bcc := make([]*pb.MailAddress, len(mail.Bcc))
		for i, addr := range mail.Bcc {
			bcc[i] = mailer.MakePbAddress(addr.Name, addr.Address)
		}
		replyTo := make([]*pb.MailAddress, len(mail.ReplyTo))
		for i, addr := range mail.ReplyTo {
			replyTo[i] = mailer.MakePbAddress(addr.Name, addr.Address)
		}

		var extraHeaders []*pb.ExtraHeader
		for k, vs := range mail.ExtraHeaders {
			for _, v := range vs {
				extraHeaders = append(extraHeaders, &pb.ExtraHeader{Key: k, Value: v})
			}
		}

		pbMail := &pb.Mail{
			Subject:     mail.Subject,
			From:        mailer.MakePbAddress(mail.From.Name, mail.From.Address),
			To:          to,
			Cc:          cc,
			Bcc:         bcc,
			ReplyTo:     replyTo,
			ExtraHeader: extraHeaders,
			BodyOneof: &pb.Mail_PlainText{
				PlainText: mail.Body,
			},
		}
		config.logger.Tracef("Converted mail to gRPC format: %+v", pbMail)

		authedCtx := ctx
		switch config.GrpcAuthMode {
		case GrpcAuthModeNone:
			break
		case GrpcAuthModeSMTPPassthrough:
			val := ctx.Value(SMTPSessionContextAuthKeyName)
			tp, ok := val.(*auth.OidcTokenProvider)
			if !ok {
				config.logger.Errorf("Failed to retrieve OIDC token provider from context for SMTP passthrough auth")
				return fmt.Errorf("failed to retrieve OIDC token provider from context for SMTP passthrough auth")
			}
			token, err := tp.GetAccessToken()
			if err != nil {
				config.logger.Errorf("Failed to get access token for gRPC request: %v", err)
				return fmt.Errorf("failed to get access token for gRPC request: %v", err)
			}
			authedCtx = metadata.AppendToOutgoingContext(authedCtx,
				"Authorization", fmt.Sprintf("Bearer %s", *token))
			logrus.Infof("Using SMTP passthrough auth mode, added Authorization header with access token to gRPC context")
		case GrpcAuthModeOIDCInject:
			accessToken, err := oidcTokenProvider.GetAccessToken()
			if err != nil {
				config.logger.Errorf("Failed to get access token for gRPC request: %v", err)
				return fmt.Errorf("failed to get access token for gRPC request: %v", err)
			}
			authedCtx = metadata.AppendToOutgoingContext(authedCtx,
				"Authorization", fmt.Sprintf("Bearer %s", *accessToken))
		default:
			config.logger.Errorf("Unknown gRPC auth mode: %s", config.GrpcAuthMode)
			return fmt.Errorf("unknown gRPC auth mode: %s", config.GrpcAuthMode)
		}

		if config.LoggingOnly {
			config.logger.Infof("Sending mail with %v", pbMail)
			return nil
		}

		config.logger.Debugf("Sending mail via gRPC with auth mode %s", config.GrpcAuthMode)

		if config.SMTPEnsureSender {
			config.logger.Tracef("Performing dry run to ensure sender address is accepted by SMTP server")
			resp, err := client.SendDryRun(authedCtx, pbMail)
			if err != nil {
				return fmt.Errorf("failed to send dry run mail via gRPC: %v", err)
			}

			respAddr := resp.From.GetMailAddress()
			if respAddr == nil {
				return fmt.Errorf("gRPC response does not contain a valid mail address")
			}

			if respAddr.Address != mail.From.Address {
				config.logger.Warnf("SMTP server rejected sender address: %s", mail.From.Address)
				return fmt.Errorf("SMTP server rejected sender address: %s", mail.From.Address)
			}
			config.logger.Debugf("Dry run successful, sender address accepted by SMTP server: %s", respAddr.Address)
		}

		resp, err := client.SendMail(authedCtx, pbMail)
		if err != nil {
			config.logger.Errorf("Failed to send mail via gRPC: %v", err)
			return fmt.Errorf("failed to send mail via gRPC: %v", err)
		}
		config.logger.Infof("Mail sent successfully via gRPC, message ID: %s", resp.MailId)

		return nil
	}

	authHandler := func(ctx context.Context, username, password string) (context.Context, error) {
		tp := auth.NewOidcTokenProvider(
			config.OidcConfig.oidcTokenEndpoint,
			username,
			password,
			config.TokenProviderOptions...,
		)

		if _, err := tp.GetAccessToken(); err != nil {
			return nil, fmt.Errorf("failed to get initial access token: %v", err)
		}

		return context.WithValue(ctx, SMTPSessionContextAuthKeyName, tp), nil
	}

	var options []smtpserver.BackendOption
	if config.SMTPAuthMode == SMTPAuthModePlain {
		options = append(options, smtpserver.WithAuthHandler(authHandler))
	}

	srv := smtp.NewServer(smtpserver.NewBackend(mailHandler, options...))
	return srv, nil
}

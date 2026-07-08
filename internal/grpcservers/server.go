package grpcservers

import (
	"github.com/sirupsen/logrus"
	pb "gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/generated/pb/sip/notifications"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/generated/sql"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/pkg/mailer"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/pkg/slack"
)

type SlackServer struct {
	loggingOnly     *bool
	unauthenticated *bool
	slackClient     *slack.Client
	logger          *logrus.Entry
	pb.UnimplementedSlackMessagingServiceServer
}

func NewSlackServer(loggingOnly, unauthenticated *bool, slackClient *slack.Client) *SlackServer {
	serverLogger := logrus.WithFields(logrus.Fields{
		"component": "slack-message-server",
	})
	return &SlackServer{
		loggingOnly:     loggingOnly,
		unauthenticated: unauthenticated,
		slackClient:     slackClient,
		logger:          serverLogger,
	}
}

type MailServer struct {
	loggingOnly     bool
	unauthenticated bool
	querier         sql.Querier
	mailSender      mailer.MailSender
	logger          *logrus.Entry
	pb.UnimplementedMailServiceServer
}

func NewMailServer(loggingOnly, unauthenticated bool, querier sql.Querier, mailSender mailer.MailSender) *MailServer {
	serverLogger := logrus.WithFields(logrus.Fields{
		"component": "mail-message-server",
	})
	return &MailServer{
		loggingOnly:     loggingOnly,
		unauthenticated: unauthenticated,
		querier:         querier,
		mailSender:      mailSender,
		logger:          serverLogger,
	}
}

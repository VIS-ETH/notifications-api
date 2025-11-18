package server

import (
	"github.com/sirupsen/logrus"
	pb "gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/generated/pb/sip/notifications"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/pkg/mailer"
)

type MailServer struct {
	loggingOnly     *bool
	unauthenticated *bool
	mailSender      *mailer.MailSender
	logger          *logrus.Entry
	pb.UnimplementedMailServiceServer
}

func NewNotificationsServer(loggingOnly, unauthenticated *bool, mailSender *mailer.MailSender) *MailServer {
	serverLogger := logrus.WithFields(logrus.Fields{
		"component": "server",
	})
	return &MailServer{
		loggingOnly:     loggingOnly,
		unauthenticated: unauthenticated,
		mailSender:      mailSender,
		logger:          serverLogger,
	}
}

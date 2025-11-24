package server

import (
	"github.com/sirupsen/logrus"
	pb "gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/generated/pb/sip/notifications"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/generated/sql"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/pkg/mailer"
)

type NotificationsServer struct {
	loggingOnly     *bool
	unauthenticated *bool
	queries         *sql.Queries
	mailSender      *mailer.MailSender
	logger          *logrus.Entry
	pb.UnimplementedMailServiceServer
}

func NewNotificationsServer(loggingOnly, unauthenticated *bool, queries *sql.Queries, mailSender *mailer.MailSender) *NotificationsServer {
	serverLogger := logrus.WithFields(logrus.Fields{
		"component": "server",
	})
	return &NotificationsServer{
		loggingOnly:     loggingOnly,
		unauthenticated: unauthenticated,
		queries:         queries,
		mailSender:      mailSender,
		logger:          serverLogger,
	}
}

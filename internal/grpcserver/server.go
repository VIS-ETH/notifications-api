package grpcserver

import (
	"github.com/sirupsen/logrus"
	pb "gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/generated/pb/sip/notifications"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/generated/sql"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/pkg/mailer"
)

type MailServer struct {
	loggingOnly     *bool
	unauthenticated *bool
	queries         *sql.Queries
	mailSender      *mailer.MailSender
	logger          *logrus.Entry
	pb.UnimplementedMailServiceServer
}

func NewMailServer(loggingOnly, unauthenticated *bool, queries *sql.Queries, mailSender *mailer.MailSender) *MailServer {
	serverLogger := logrus.WithFields(logrus.Fields{
		"component": "server",
	})
	return &MailServer{
		loggingOnly:     loggingOnly,
		unauthenticated: unauthenticated,
		queries:         queries,
		mailSender:      mailSender,
		logger:          serverLogger,
	}
}

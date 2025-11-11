package server

import (
	"github.com/sirupsen/logrus"
	pb "gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/generated/pb/sip/notifications"
)

type NotificationsServer struct {
	defaultMailSenderAddress *string
	smtpEndpoint             *string
	mailLogger               *logrus.Logger
	pb.UnimplementedNotificationsServiceServer
}

func NewNotificationsServer(defaultMailSenderAddress string, smtpEndpoint *string) *NotificationsServer {
	mailLogger := logrus.WithFields(logrus.Fields{
		"component": "mail",
	})
	return &NotificationsServer{
		defaultMailSenderAddress: &defaultMailSenderAddress,
		smtpEndpoint:             smtpEndpoint,
		mailLogger:               mailLogger.Logger,
	}
}

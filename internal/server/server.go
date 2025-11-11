package server

import (
	"github.com/sirupsen/logrus"
	pb "gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/generated/pb/sip/notifications"
)

type NotificationsServer struct {
	loggingOnly     *bool
	unauthenticated *bool
	mailConfig      *MailConfiguration
	pb.UnimplementedNotificationsServiceServer
}

type MailConfiguration struct {
	defaultMailSenderAddress string
	smtpEndpoint             string
	logger                   *logrus.Logger
}

func NewNotificationsServer(loggingOnly, unauthenticated *bool, mailConfig *MailConfiguration) *NotificationsServer {
	return &NotificationsServer{
		loggingOnly:     loggingOnly,
		unauthenticated: unauthenticated,
		mailConfig:      mailConfig,
	}
}

func NewMailConfiguration(defaultSender string, smtpEndpoint string) *MailConfiguration {
	mailLogger := logrus.WithFields(logrus.Fields{
		"component": "mail",
	})

	return &MailConfiguration{
		defaultMailSenderAddress: defaultSender,
		smtpEndpoint:             smtpEndpoint,
		logger:                   mailLogger.Logger,
	}
}

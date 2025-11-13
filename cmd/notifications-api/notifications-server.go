package main

import (
	"context"
	"flag"
	"net"
	"os"
	"strings"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/sirupsen/logrus"
	pb "gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/generated/pb/sip/notifications"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal/auth"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal/server"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/pkg/mailer"
	"google.golang.org/grpc"
)

func main() {
	logrus.Info("Starting Notifications API")

	// SMTP Server
	smtpEndpoint := flag.String(
		"smtp-url",
		EnvOrDefault("SMTP_MAIL_URL", "localhost:2225"),
		"SMTP URL for mail client",
	)

	// Auth flags
	oidcClientID := flag.String(
		"oidc-client-id",
		EnvOrDefault("SIP_AUTH_OIDC_CLIENT_ID", "notifications-api"),
		"Client ID used for Notifications API",
	)
	oidcIssuer := flag.String(
		"oidc-issuer",
		EnvOrDefault("SIP_AUTH_OIDC_ISSUER", "https://keycloak-fake.vis.ethz.ch/realms/VSETH"),
		"Issuer URL for OIDC",
	)
	oidcJwksURL := flag.String(
		"oidc-client-jwks-url",
		EnvOrDefault("SIP_AUTH_OIDC_JWKS_URL", "https://keycloak-fake.vis.ethz.ch/realms/VSETH/protocol/openid-connect/certs"),
		"Client ID used for Notifications API",
	)

	// gRPC server config
	// Notifications Server configuration
	loggingOnly := flag.Bool(
		"grpc-logging-only",
		// "local-testing" first mentality...
		strings.ToLower(EnvOrDefault("NOTIFICATIONS_LOGGING_ONLY", "true")) == "true",
		"Only log notifications, without sending",
	)
	unauthenticatedGrpc := flag.Bool(
		"grpc-unauthenticated",
		strings.ToLower(EnvOrDefault("NOTIFICATIONS_UNAUTHENTICATED", "false")) == "true",
		"Skip authentication checks on incoming gRPC requests",
	)
	addrFlag := flag.String(
		"grpc-addr",
		EnvOrDefault("NOTIFICATIONS_BACKEND_GRPC_PORT", ":6781"),
		"gRPC listen address",
	)

	flag.Parse()

	logrus.Infof("Starting Notifications API with parameters: %+v", map[string]any{
		"Unauthenticated gRPC": *unauthenticatedGrpc,
		"Logging only":         *loggingOnly,
		"gRPC server address":  *addrFlag,
		"SMTP endpoint":        *smtpEndpoint,
	})

	logLevelEnv := EnvOrDefault("LOG_LEVEL", "info")
	logLevel, err := logrus.ParseLevel(logLevelEnv)
	if err != nil {
		logrus.Fatalf("Failed to set log level: %v", err)
	}
	logrus.SetLevel(logLevel)

	k, err := keyfunc.NewDefaultCtx(context.Background(), []string{*oidcJwksURL})
	if err != nil {
		logrus.Fatalf("Failed to create a keyfunc.Keyfunc from the server's URL. Error: %s", err)
	}

	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(auth.GetGrpcAuthInterceptor(oidcIssuer, oidcClientID, unauthenticatedGrpc, k.Keyfunc)),
	)

	mailConfig := mailer.NewMailSender(
		"serviceaccount@vis.ethz.ch",
		*smtpEndpoint,
	)

	notificationsServer := server.NewNotificationsServer(
		loggingOnly,
		unauthenticatedGrpc,
		mailConfig,
	)

	pb.RegisterNotificationsServiceServer(grpcServer, notificationsServer)
	l, err := net.Listen("tcp", *addrFlag)
	if err != nil {
		logrus.Fatalf("Failed to listen: %v", err)
	}
	logrus.Printf("Serving gRPC at %s", l.Addr().String())

	logrus.Fatalf("Failed to serve: %v", grpcServer.Serve(l))
}

func EnvOrDefault(envVar, defaultVal string) string {
	envVal, exists := os.LookupEnv(envVar)
	if exists {
		return envVal
	}
	return defaultVal
}

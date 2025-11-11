package main

import (
	"context"
	"flag"
	"net"
	"os"

	"github.com/MicahParks/keyfunc/v3"
	log "github.com/sirupsen/logrus"
	pb "gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/generated/pb/sip/notifications"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal/auth"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal/server"
	"google.golang.org/grpc"
)

func main() {
	log.Info("Start")

	// SMTP Server
	smtpEndpoint := flag.String(
		"smtp-url",
		EnvOrDefault("SMTP_MAIL_URL", "localhost:2225"),
		"SMTP URL for mail client",
	)

	// Auth flags
	oidcClientID := flag.String(
		"oidc-client-id",
		EnvOrDefault("SIP_AUTH_OIDC_CLIENT_ID", "cdn-backend"),
		"Client ID used for CDN",
	)
	oidcIssuer := flag.String(
		"oidc-issuer",
		EnvOrDefault("SIP_AUTH_OIDC_ISSUER", "https://keycloak-fake.vis.ethz.ch/realms/VSETH"),
		"Issuer URL for OIDC",
	)
	oidcJwksURL := flag.String(
		"oidc-client-jwks-url",
		EnvOrDefault("SIP_AUTH_OIDC_JWKS_URL", "https://keycloak-fake.vis.ethz.ch/realms/VSETH/protocol/openid-connect/certs"),
		"Client ID used for CDN",
	)

	// gRPC server config
	addrFlag := flag.String(
		"addr",
		EnvOrDefault("CDN_BACKEND_GRPC_PORT", ":6781"),
		"gRPC listen address",
	)

	k, err := keyfunc.NewDefaultCtx(context.Background(), []string{*oidcJwksURL})
	if err != nil {
		log.Fatalf("Failed to create a keyfunc.Keyfunc from the server's URL. Error: %s", err)
	}

	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(auth.GetGrpcAuthInterceptor(oidcIssuer, oidcClientID, k.Keyfunc)),
	)

	notificationsServer := server.NewNotificationsServer(
		"serviceaccount@vis.ethz.ch",
		smtpEndpoint,
	)

	pb.RegisterNotificationsServiceServer(grpcServer, notificationsServer)
	l, err := net.Listen("tcp", *addrFlag)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}
	log.Printf("Serving gRPC at %s", l.Addr().String())

	log.Fatalf("Failed to serve: %v", grpcServer.Serve(l))
}

func EnvOrDefault(envVar, defaultVal string) string {
	envVal, exists := os.LookupEnv(envVar)
	if exists {
		return envVal
	}
	return defaultVal
}

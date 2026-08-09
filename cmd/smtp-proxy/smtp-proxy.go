package main

import (
	"crypto/tls"
	"log"

	pb "gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/generated/pb/sip/notifications"
	smtpproxy "gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal/smtp-proxy"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	clientConn, err := grpc.NewClient("localhost:6781",
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("failed to connect to gRPC server: %v", err)
	}

	client := pb.NewMailServiceClient(clientConn)
	srv := smtpproxy.GetSMTPServer(client)
	srv.Addr = ":2225"
	srv.Domain = "localhost"
	srv.EnableREQUIRETLS = false
	srv.TLSConfig = &tls.Config{}
	srv.AllowInsecureAuth = true

	err = srv.ListenAndServe()
	if err != nil {
		log.Fatalf("serving SMTP server failed: %v", err)
	}
}

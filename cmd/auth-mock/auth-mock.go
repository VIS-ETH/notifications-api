package main

import (
	"flag"
	"log"
	"net/http"

	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal"
	authmock "gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal/auth/mock"
)

func main() {
	addrFlag := flag.String(
		"listen-addr",
		internal.EnvOrDefault("LISTEN_ADDR", ":8081"),
		"listen address",
	)
	issuerURLFlag := flag.String(
		"issuer-url",
		internal.EnvOrDefault("ISSUER_URL", "localhost:8081"),
		"issuer URL for the mock auth server",
	)
	handle, err := authmock.NewAuthMockServerHandle(authmock.AuthMockUserUsername, authmock.AuthMockUserPassword)
	if err != nil {
		log.Fatalf("Failed to start mock auth server: %v", err)
	}

	srvListen, err := handle.StartHttpMockServer(*addrFlag, *issuerURLFlag)
	if err != nil {
		log.Fatalf("Failed to start mock auth server: %v", err)
	}

	err = srvListen()
	if err != nil && err != http.ErrServerClosed {
		log.Fatalf("Mock server failed to serve: %v", err)
	}

	log.Printf("Mock auth server running at: %s", handle.URL())
}

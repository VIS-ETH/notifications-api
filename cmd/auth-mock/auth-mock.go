package main

import (
	"flag"
	"log"
	"net/http"

	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal/auth"
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
	handle, err := auth.NewAuthMockServerHandle(auth.AuthMockUserUsername, auth.AuthMockUserPassword)
	if err != nil {
		log.Fatalf("Failed to start mock auth server: %v", err)
	}

	srvListen, err := auth.StartHttpMockServer(
		*addrFlag,
		*issuerURLFlag,
		handle)
	if err != nil {
		log.Fatalf("Failed to start mock auth server: %v", err)
	}

	err = srvListen()
	if err != nil && err != http.ErrServerClosed {
		log.Fatalf("Mock server failed to serve: %v", err)
	}

	log.Printf("Mock auth server running at: %s", handle.URL())
}

package main

import (
	"flag"
	"log"
	"net"
	"os"

	pb "gitlab.ethz.ch/vseth/1100-fv/1116-vis/cat/sip-vis-cat-apps/2message2api/internal/pb/servis/self"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cat/sip-vis-cat-apps/2message2api/internal/server"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cat/sip-vis-cat-apps/2message2api/internal/slack"
	"google.golang.org/grpc"
)

var (
	slackSecret = flag.String("slack-secret", os.Getenv("RUNTIME_SLACK_API_KEY"), "slack message-api key")
	port        = flag.String("port", os.Getenv("RUNTIME_SERVIS_SELF_PORT"), "runtime grpc port")
)

func main() {
	flag.Parse()
	srv := grpc.NewServer()
	slackClient := slack.NewClient(*slackSecret)
	pb.RegisterMessengerServer(srv, server.NewMessengerServer(slackClient))

	lis, err := net.Listen("tcp", ":"+*port)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("listening on %s", lis.Addr())
	if err := srv.Serve(lis); err != nil {
		log.Fatalf("grpc serve failed: %v", err)
	}
}

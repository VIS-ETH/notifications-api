package server

import (
	"context"

	pb "gitlab.ethz.ch/vseth/1100-fv/1116-vis/cat/sip-vis-cat-apps/2message2api/internal/pb/servis/vseth/vis/messaging"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cat/sip-vis-cat-apps/2message2api/internal/slack"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type MessagingServer struct {
	pb.UnimplementedMessagingServer
	slackClient *slack.Client
}

func NewMessagingServer(sc *slack.Client) *MessagingServer {
	return &MessagingServer{slackClient: sc}
}

func (s *MessagingServer) SendSlackMessage(ctx context.Context, req *pb.SlackRequest) (*pb.SlackResponse, error) {
	if err := s.slackClient.Send(ctx, req.Recipient, req.BlocksJson, req.FallbackText); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to send Slack message: %v", err)
	}
	return nil, nil
}

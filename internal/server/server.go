package server

import (
	"context"

	pb "gitlab.ethz.ch/vseth/1100-fv/1116-vis/cat/sip-vis-cat-apps/2message2api/internal/pb/servis/self"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cat/sip-vis-cat-apps/2message2api/internal/slack"
)

type MessengerServer struct {
	pb.UnimplementedMessengerServer
	slackClient *slack.Client
}

func NewMessengerServer(sc *slack.Client) *MessengerServer {
	return &MessengerServer{slackClient: sc}
}

func (s *MessengerServer) SendMessage(ctx context.Context, req *pb.SendRequest) (*pb.SendResponse, error) {
	if err := s.slackClient.Send(ctx, req.Email, req.Text); err != nil {
		// TODO so here we say nah we are totally fine lmao
		return &pb.SendResponse{Success: false, Error: err.Error()}, nil
	}
	return &pb.SendResponse{Success: true}, nil
}

package server

import (
	"2message2api/internal/pb"
	"2message2api/internal/slack"
	"context"
)

type MessagingServer struct {
	pb.UnimplementedMessagingServer
	slackClient *slack.Client
}

func NewMessengerServer(sc *slack.Client) *MessagingServer {
	return &MessagingServer{slackClient: sc}
}

func (s *MessagingServer) SendMessage(ctx context.Context, req *pb.SendRequest) (*pb.SendRespone, error) {
	if err := s.slackClient.Send(ctx, req.Email, req.Text); err != nil {
		// TODO so here we say nah we are totally fine lmao
		return &pb.SendResponse{Success: false, Error: err.Error()}, nil
	}
	return &pb.SendRespone{Success: true}, nil
}

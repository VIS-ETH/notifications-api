package grpcservers

import (
	"context"

	pb "gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/generated/pb/sip/notifications"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func (s *SlackServer) SendSlackMessage(ctx context.Context, req *pb.SlackRequest) (*pb.SlackResponse, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, status.Errorf(codes.Unauthenticated, "No metadata could be extracted from request")
	}

	if *s.loggingOnly {
		s.logger.Infof("Sending message %+v", req)
		return &pb.SlackResponse{}, nil
	}

	headers := md.Get("X-Slack-Token")
	if len(headers) == 0 {
		return nil, status.Errorf(codes.Unauthenticated, "No X-Slack-Token header found in request")
	} else if len(headers) > 1 {
		return nil, status.Errorf(codes.Unauthenticated, "Multiple X-Slack-Token found in request")
	}
	token := headers[0]

	if err := s.slackClient.Send(ctx, token, req.Recipient, req.BlocksJson, req.FallbackText); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to send Slack message: %v", err)
	}
	return &pb.SlackResponse{}, nil
}

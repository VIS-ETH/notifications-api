srv := grpc.NewServer()
pb.RegisterMessengerServer(srv, server.NewMessengerServer(slackClient))
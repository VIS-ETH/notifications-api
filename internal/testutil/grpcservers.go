package testutil

import (
	"context"
	"fmt"
	"net"
	"net/http/httptest"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/sirupsen/logrus"
	mockery_sql "gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/generated/mockery/generated/sql"
	mockery_mailer "gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/generated/mockery/pkg/mailer"
	pb "gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/generated/pb/sip/notifications"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal/auth"
	authmock "gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal/auth/mock"
	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal/grpcservers"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type MockMailGrpcServer struct {
	Querier    *mockery_sql.MockQuerier
	Mailer     *mockery_mailer.MockMailSender
	GrpcServer *grpc.Server
}

func (te *TestEnvironment) GetMockMailerGRPCServer(authMockSrv *httptest.Server) MockMailGrpcServer {
	querier := mockery_sql.NewMockQuerier(te.T)
	mockMailer := mockery_mailer.NewMockMailSender(te.T)

	var opts []grpc.ServerOption
	if authMockSrv != nil {
		k, err := keyfunc.NewDefaultCtx(context.Background(), []string{
			authMockSrv.URL + authmock.AuthMockServerJWKSPath,
		})
		if err != nil {
			logrus.Fatalf("Failed to create a keyfunc.Keyfunc from the server's URL. Error: %v", err)
		}
		opts = append(opts, grpc.UnaryInterceptor(auth.GetGrpcAuthInterceptor(authMockSrv.URL, authmock.AuthMockClientID, k.Keyfunc)))
	}

	grpcServer := grpc.NewServer(opts...)

	mailGrpcServer := grpcservers.NewMailServer(
		false,
		authMockSrv == nil,
		querier,
		mockMailer,
	)

	pb.RegisterMailServiceServer(grpcServer, mailGrpcServer)

	return MockMailGrpcServer{
		Querier:    querier,
		Mailer:     mockMailer,
		GrpcServer: grpcServer,
	}
}

func (te *TestEnvironment) StartGRPCMailServer(grpcServer *grpc.Server) pb.MailServiceClient {
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		te.T.Fatalf("Failed to listen: %v", err)
	}

	te.Eg.Go(func() error {
		te.T.Logf("Serving gRPC at %s", l.Addr().String())
		err := grpcServer.Serve(l)
		if err != nil {
			return fmt.Errorf("failed to serve: %v", err)
		}
		return nil
	})
	te.Eg.Go(func() error {
		<-te.Ctx.Done()
		grpcServer.GracefulStop()
		return nil
	})

	clientConn, err := grpc.NewClient(l.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		te.T.Fatalf("failed to connect to gRPC server: %v", err)
	}
	client := pb.NewMailServiceClient(clientConn)

	return client
}

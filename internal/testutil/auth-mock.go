package testutil

import (
	"net/http/httptest"

	authmock "gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal/auth/mock"
)

func (te *TestEnvironment) StartAuthMockServer() *httptest.Server {
	authMockSrvHandle, err := authmock.NewAuthMockServerHandle(authmock.AuthMockUserUsername, authmock.AuthMockUserPassword)
	if err != nil {
		te.T.Fatalf("Failed to start auth server: %v", err)
	}
	authMockSrv, err := authMockSrvHandle.StartHttpTestServer(te.T)
	if err != nil {
		te.T.Fatalf("Could not start mock server: %v", err)
	}

	te.Eg.Go(func() error {
		<-te.Ctx.Done()
		authMockSrv.Close()
		return nil
	})

	te.T.Logf("Auth server started at %s", authMockSrv.URL)
	return authMockSrv
}

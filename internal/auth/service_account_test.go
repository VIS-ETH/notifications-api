package auth_test

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"testing"
	"testing/synctest"
	"time"

	"gitlab.ethz.ch/vseth/1100-fv/1116-vis/cit/sip-vis-cit-apps/notifications-api/internal/auth"
)

const (
	TestTokenEndpoint     = "http://test-token-endpoint"
	TestTokenEndpointHost = "test-token-endpoint"
	TestClientID          = "test-client-id"
	TestClientSecret      = "test-client-secret"
	TestClientScope       = "test-c://lient-scope"
	TestExpiresIn         = 1 * time.Hour
)

type RoundTripFunc func(req *http.Request) (*http.Response, error)

func (f RoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func testWithChecks(
	t *testing.T,
	requestHandler func(*http.Request) (*http.Response, error),
	responseChecker func(accessToken *string, err error) error,
	options ...auth.OidcTokenProviderOption,
) {
	bareMinimumClaims := fmt.Appendf(nil, `{"exp": %d}`, time.Now().Add(TestExpiresIn).UnixMilli()/1000)
	encodedExpiration := base64.RawURLEncoding.EncodeToString(bareMinimumClaims)
	testAccessToken := fmt.Sprintf("abc.%s.def", encodedExpiration)

	fakeProvideToken := func(req *http.Request) (*http.Response, error) {
		resp, err := requestHandler(req)
		if err != nil {
			t.Errorf("Failed with %v", err)
		}

		if t.Failed() {
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Status:     "bad request",
			}, nil
		}

		if resp == nil {
			resp = &http.Response{
				StatusCode: 200,
				Status:     "OK",
				Body:       io.NopCloser(bytes.NewBufferString(fmt.Sprintf(`{"access_token": "%s"}`, testAccessToken))),
			}
		}

		return resp, nil
	}

	httpClient := &http.Client{
		Transport: RoundTripFunc(fakeProvideToken),
	}

	options = append(options, auth.WithHTTPClient(httpClient))

	tp := auth.NewOidcTokenProvider(
		TestTokenEndpoint, TestClientID, TestClientSecret,
		options...,
	)

	accessToken, err := tp.GetAccessToken()
	err = responseChecker(accessToken, err)
	if err != nil {
		t.Errorf("Response not as expected %v", err)
	}
}

func TestEmptyNoRequest(t *testing.T) {
	fakeProvideToken := func(req *http.Request) (*http.Response, error) {
		t.Error("Should not be called...")
		return nil, errors.New("no response")
	}

	httpClient := &http.Client{
		Transport: RoundTripFunc(fakeProvideToken),
	}

	tp := auth.NewOidcTokenProvider(
		TestTokenEndpoint, "", "",
		auth.WithHTTPClient(httpClient),
	)

	accessToken, err := tp.GetAccessToken()
	if err != nil {
		t.Errorf("Response not as expected %v", err)
		return
	}
	if accessToken == nil || *accessToken != "" {
		t.Errorf("expecting empty string if no credentials given!")
		return
	}

	tp = auth.NewOidcTokenProvider(
		TestTokenEndpoint, "", "",
	)

	accessToken, err = tp.GetAccessToken()
	if err != nil {
		t.Errorf("Response not as expected %v", err)
		return
	}
	if accessToken == nil || *accessToken != "" {
		t.Errorf("expecting empty string if no credentials given!")
	}
}

func TestFailures(t *testing.T) {
	t.Run("check jwt dots wrong", func(t *testing.T) {
		bareMinimumClaims := fmt.Appendf(nil, `{"exp": %d}`, time.Now().Add(TestExpiresIn).UnixMilli()/1000)
		encodedExpiration := base64.RawURLEncoding.EncodeToString(bareMinimumClaims)
		accessToken := fmt.Sprintf("abc.%s.def.", encodedExpiration)
		incorrectTokens := []string{"test", "qoqwoiejoiqwe..", "tqow.qwe.e.", "a.b.c", "", accessToken + "."}

		var incorrectValues []string
		for _, value := range incorrectTokens {
			incorrectValues = append(incorrectValues, fmt.Sprintf(`{"access_token": "%s"}`, value))
		}
		incorrectValues = append(incorrectValues, `{}`, `{"access_token":[]}`, fmt.Sprintf(`{"AccessToken": %s}`, accessToken))

		for _, incorrectValue := range incorrectValues {
			testWithChecks(t, func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "OK",
					Body:       io.NopCloser(bytes.NewBufferString(incorrectValue)),
				}, nil
			}, func(accessToken *string, err error) error {
				if err == nil {
					return fmt.Errorf("Expected failure because JWT token has incorrect format")
				} else if accessToken != nil {
					return fmt.Errorf("This should be nil in case of failures")
				}
				return nil
			})
		}
	})

	t.Run("check error response propagates", func(t *testing.T) {
		testWithChecks(t, func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Status:     "bad request",
				Body:       io.NopCloser(bytes.NewBufferString("check fail if upstream fail")),
			}, nil
		}, func(accessToken *string, err error) error {
			if err == nil {
				return fmt.Errorf("Expected failure because response failed")
			} else if accessToken != nil {
				return fmt.Errorf("This should be nil in case of failures")
			}
			return nil
		})
	})

	t.Run("check error fail propagates", func(t *testing.T) {
		fakeProvideToken := func(req *http.Request) (*http.Response, error) {
			return nil, errors.New("intentional error")
		}

		httpClient := &http.Client{
			Transport: RoundTripFunc(fakeProvideToken),
		}

		tp := auth.NewOidcTokenProvider(
			TestTokenEndpoint, TestClientID, TestClientSecret,
			auth.WithHTTPClient(httpClient),
		)

		_, err := tp.GetAccessToken()
		if err == nil {
			t.Errorf("needs to fail")
		}
	})

	t.Run("check error request create", func(t *testing.T) {
		fakeProvideToken := func(req *http.Request) (*http.Response, error) {
			t.Error("should not even get here")
			return nil, errors.New("should not even get here")
		}

		httpClient := &http.Client{
			Transport: RoundTripFunc(fakeProvideToken),
		}

		tp := auth.NewOidcTokenProvider(
			"test-%91240!924+%`-endpoint", TestClientID, TestClientSecret,
			auth.WithHTTPClient(httpClient),
		)

		_, err := tp.GetAccessToken()
		if err == nil {
			t.Errorf("needs to fail")
		}
	})
}

func TestTokenEndpointScope(t *testing.T) {
	testWithChecks(
		t, func(req *http.Request) (*http.Response, error) {
			query := req.URL.Query()
			scopes := query["scope"]

			if len(scopes) != 1 {
				return nil, fmt.Errorf("Scope was expected to not be empty, found %d instances", len(scopes))
			} else if actualScope := query.Get("scope"); TestClientScope != actualScope {
				return nil, fmt.Errorf("Scope was expected to be '%s', not '%s'", TestClientScope, actualScope)
			} else if req.URL.Host != TestTokenEndpointHost {
				return nil, fmt.Errorf("Host was expected to be '%s', was '%s'", TestTokenEndpointHost, req.URL.Host)
			}

			return nil, nil
		}, func(accessToken *string, err error) error {
			if err != nil {
				return fmt.Errorf("Failed to get access token: %v", err)
			}

			return nil
		},
		auth.WithScope(TestClientScope),
	)
}

func TestTokenProviderRefreshing(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		bareMinimumClaims := fmt.Appendf(nil, `{"exp": %d}`, time.Now().Add(TestExpiresIn).UnixMilli()/1000)
		encodedExpiration := base64.RawURLEncoding.EncodeToString(bareMinimumClaims)

		testAccessToken := fmt.Sprintf("abc.%s.def", encodedExpiration)
		var lastRequested time.Time

		// https://go-review.googlesource.com/c/go/+/769521
		// Requires go 1.27
		/*
			testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
				if time.Now().Before(lastRequested.Add(1 * time.Hour)) {
					t.Errorf("Requested token, even though previous request provided token has not expired (is %s, requested at %s)", time.Now(), lastRequested)
					res.WriteHeader(http.StatusBadRequest)
					_, err = res.Write(fmt.Appendf(nil, "fail"))
				} else {
					_, err = res.Write(fmt.Appendf(nil, `
				{"access_token": "%s"}
				`, testAccessToken))
				}

				if err != nil {
					t.Error("Failed to write mock response")
				}
				lastRequested = time.Now()
			}))
		*/

		madeTokenRequest := false

		fakeProvideToken := func(req *http.Request) (*http.Response, error) {
			madeTokenRequest = true
			if time.Now().Before(lastRequested.Add(1 * time.Hour)) {
				t.Errorf("Requested token, even though previous request provided token has not expired (is %s, requested at %s)", time.Now(), lastRequested)
				return &http.Response{
					StatusCode: http.StatusBadRequest,
					Body:       io.NopCloser(bytes.NewBufferString("failed - requested token too early")),
				}, nil
			}

			lastRequested = time.Now()
			return &http.Response{
				StatusCode: 200,
				Status:     "OK",
				Body:       io.NopCloser(bytes.NewBufferString(fmt.Sprintf(`{"access_token": "%s"}`, testAccessToken))),
			}, nil
		}

		httpClient := &http.Client{
			Transport: RoundTripFunc(fakeProvideToken),
		}

		tp := auth.NewOidcTokenProvider(
			TestTokenEndpoint, TestClientID, TestClientSecret,
			auth.WithScope(TestClientScope),
			auth.WithHTTPClient(httpClient),
		)

		testRefreshMakeAndCheckRequest := func() {
			madeTokenRequest = false
			accessToken, err := tp.GetAccessToken()
			if err != nil {
				t.Errorf("Failed to get access token, encountered error: %v", err)
				return
			}
			if accessToken == nil || *accessToken != testAccessToken {
				t.Error("Received access token not as expected")
			}
		}

		testRefreshMakeAndCheckRequest()
		if !madeTokenRequest {
			t.Error("This succeeded without making request... impossible")
		}

		time.Sleep(5 * time.Second)
		synctest.Wait()

		testRefreshMakeAndCheckRequest()
		if madeTokenRequest {
			t.Error("No request should have been made after 5 seconds")
		}

		time.Sleep(1 * time.Hour)
		synctest.Wait()

		testRefreshMakeAndCheckRequest()
		if !madeTokenRequest {
			t.Error("Request should have been made after more than an hour")
		}
	})
}

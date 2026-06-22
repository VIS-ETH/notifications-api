package auth

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

type ServiceAccountCredential struct {
	ClientID     string
	ClientSecret string
}

type OidcTokenProvider struct {
	tokenEndpoint string
	sa            *ServiceAccountCredential
	httpClient    *http.Client
	scope         string
	accessToken   *string
	expiry        *int64
}

type OidcTokenProviderOption func(*OidcTokenProvider)

func NewOidcTokenProvider(tokenEndpoint, clientID, clientSecret string, options ...OidcTokenProviderOption) *OidcTokenProvider {
	tp := &OidcTokenProvider{
		tokenEndpoint: tokenEndpoint,
		sa: &ServiceAccountCredential{
			ClientID:     clientID,
			ClientSecret: clientSecret,
		},
		scope: "openid",
	}

	for _, option := range options {
		option(tp)
	}

	if tp.httpClient == nil {
		tp.httpClient = &http.Client{}
	}

	return tp
}

func WithScope(scope string) OidcTokenProviderOption {
	return func(tp *OidcTokenProvider) {
		tp.scope = scope
	}
}

func WithHTTPClient(httpClient *http.Client) OidcTokenProviderOption {
	return func(tp *OidcTokenProvider) {
		tp.httpClient = httpClient
	}
}

func getExpiryTimestamp(jwt string) (int64, error) {
	parts := strings.Split(jwt, ".")

	if len(parts) != 3 {
		return 0, errors.New("invalid jwt token")
	}

	decoded, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return 0, err
	}

	type jwtTokenPayload struct {
		ExpiryTimestamp int64 `json:"exp"`
	}

	payload := jwtTokenPayload{}
	err = json.Unmarshal(decoded, &payload)
	if err != nil {
		return 0, err
	}

	return payload.ExpiryTimestamp, nil
}

func (tp *OidcTokenProvider) refreshAccessToken() error {
	req, err := http.NewRequest("POST", tp.tokenEndpoint, bytes.NewBuffer([]byte("grant_type=client_credentials")))
	if err != nil {
		return fmt.Errorf("failed to create a http request to (%s): %v", tp.tokenEndpoint, err)
	}

	encodedCredentials := base64.StdEncoding.EncodeToString(fmt.Appendf(nil, "%s:%s", tp.sa.ClientID, tp.sa.ClientSecret))

	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("Authorization", fmt.Sprintf("Basic %s", encodedCredentials))

	query := req.URL.Query()
	if tp.scope != "" {
		query.Add("scope", tp.scope)
	}
	req.URL.RawQuery = query.Encode()

	resp, err := tp.httpClient.Do(req)
	if err != nil {
		log.Printf("failed to create a obtain access token from (%s) for client (%s): %v", tp.tokenEndpoint, tp.sa.ClientID, err)
		return errors.New("failed to retrieve access token")
	}
	defer func() { err = errors.Join(err, resp.Body.Close()) }()

	type AuthResponse struct {
		AccessToken string `json:"access_token"`
	}

	auth := AuthResponse{}
	err = json.NewDecoder(resp.Body).Decode(&auth)
	if err != nil {
		return err
	}

	token := auth.AccessToken
	expiry, err := getExpiryTimestamp(token)
	if err != nil {
		return err
	}

	tp.accessToken = &token
	tp.expiry = &expiry

	return nil
}

func (tp *OidcTokenProvider) GetAccessToken() (*string, error) {
	if tp.sa == nil || (tp.sa.ClientID == "" && tp.sa.ClientSecret == "") {
		log.Printf("Not using authentication for service!")
		empty := ""
		return &empty, nil
	}
	if tp.accessToken == nil || tp.expiry == nil || (*tp.expiry-5 < time.Now().Unix()) {
		err := tp.refreshAccessToken()
		if err != nil {
			return nil, err
		}
	}

	return tp.accessToken, nil
}

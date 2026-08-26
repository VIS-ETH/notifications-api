package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/sirupsen/logrus"
)

type ListenFunc func() error

const (
	AuthMockServerJWKSPath  = "/jwks"
	AuthMockServerTokenPath = "/token"

	AuthMockUserUsername = "test-usernam3"
	AuthMockUserPassword = "test-passw0rd"
	AuthMockUserSub      = "test-5ub"

	AuthMockClientID = "test-client-id"
)

type AuthMockServerHandle struct {
	handler    http.Handler
	privateKey *rsa.PrivateKey
	logger     *logrus.Entry
	url        string
}

func (h *AuthMockServerHandle) URL() string {
	return h.url
}

func StartHttpTestServer(mockServerHandler *AuthMockServerHandle) (*httptest.Server, error) {
	if url := mockServerHandler.url; url != "" {
		return nil, fmt.Errorf("mock server already started at %s", url)
	}
	srv := httptest.NewServer(mockServerHandler.handler)
	mockServerHandler.url = srv.URL
	return srv, nil
}

func StartHttpMockServer(addr string, mockServerHandler *AuthMockServerHandle) (ListenFunc, error) {
	if url := mockServerHandler.url; url != "" {
		return nil, fmt.Errorf("mock server already started at %s", url)
	}
	srv := &http.Server{
		Addr:    addr,
		Handler: mockServerHandler.handler,
	}

	l, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on %s: %v", addr, err)
	}
	mockServerHandler.url = fmt.Sprintf("http://%s", l.Addr().String())

	return func() error {
		return srv.Serve(l)
	}, nil
}

func NewAuthMockServerHandle(username, password string) (*AuthMockServerHandle, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("Failed to generate a secure random key for the auth jwt testing server: %v", err)
	}
	publicKey := &privateKey.PublicKey

	logger := logrus.WithFields(logrus.Fields{
		"component": "auth-mock-server",
	})

	handle := &AuthMockServerHandle{
		logger:     logger,
		privateKey: privateKey,
	}

	httpHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/jwks":
			jwks := map[string]any{
				"keys": []map[string]any{
					{
						"kty": "RSA",
						"kid": "test-key-id",
						"alg": "RS256",
						"use": "sig",
						"n":   base64.RawURLEncoding.EncodeToString(publicKey.N.Bytes()),
						"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(publicKey.E)).Bytes()),
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			err = json.NewEncoder(w).Encode(jwks)
		case "/token":
			if l := len(r.Header.Values("Authorization")); l != 1 {
				w.WriteHeader(http.StatusBadRequest)
				_, err = fmt.Fprintf(w, "Expected 1 Authorization header, got %d", l)
				if err != nil {
					logger.Errorf("Failed to write response: %v", err)
				}
				break
			}
			encodedUsernamePassword := r.Header.Get("Authorization")
			if !strings.HasPrefix(encodedUsernamePassword, "Basic ") {
				w.WriteHeader(http.StatusBadRequest)
				_, err = fmt.Fprintf(w, "Expected Authorization header to start with 'Basic ', got '%s'", encodedUsernamePassword)
				if err != nil {
					logger.Errorf("Failed to write response: %v", err)
				}
				break
			}
			b, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(encodedUsernamePassword, "Basic "))
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				_, err = fmt.Fprintf(w, "Failed to decode Authorization header: %v", err)
				if err != nil {
					logger.Errorf("Failed to write response: %v", err)
				}
			}
			body := string(b)
			if body != fmt.Sprintf("%s:%s", username, password) {
				w.WriteHeader(http.StatusUnauthorized)
				_, err = fmt.Fprintf(w, "Expected Authorization header to be '%s:%s', got '%s'", username, password, body)
				if err != nil {
					logger.Errorf("Failed to write response: %v", err)
				}
				break
			}
			token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
				"sub": AuthMockUserSub,
				"iss": handle.url,
				"aud": AuthMockClientID,
				"resource_access": map[string]map[string][]string{
					AuthMockClientID: {
						"roles": {
							"mail",
							"mail-sender:test-from@local",
						},
					},
				},
				"exp": time.Now().Add(time.Hour).Unix(),
			})
			token.Header["kid"] = "test-key-id"

			signedToken, err := token.SignedString(privateKey)
			if err != nil {
				http.Error(w, "Failed to sign token", http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			err = json.NewEncoder(w).Encode(map[string]any{
				"access_token": signedToken,
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
			if err != nil {
				logger.Errorf("Failed to write response: %v", err)
			}

			logger.Errorf("Auth server issued token: %s", signedToken)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	handle.handler = httpHandler

	return handle, nil
}

package tests_test

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/sirupsen/logrus"
)

const (
	TestAuthServerJWKSPath  = "/jwks"
	TestAuthServerTokenPath = "/token"

	TestAuthUsername = "test-usernam3"
	TestAuthPassword = "test-passw0rd"
	TestAuthSub      = "test-5ub"
)

func GetAuthServer(username, password string) (*httptest.Server, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("Failed to generate a secure random key for the auth jwt testing server: %v", err)
	}
	publicKey := &privateKey.PublicKey

	var authServer *httptest.Server

	logger := logrus.WithFields(logrus.Fields{
		"component": "auth-mock-server",
	})

	authServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			// Create a valid JWT using the exact same private key
			token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
				"sub": "test-sub",
				"iss": authServer.URL,
				"aud": TestClientID,
				"resource_access": map[string]map[string][]string{
					TestClientID: {
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

			// Return the standard OAuth2 token response
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
	}))

	return authServer, nil
}

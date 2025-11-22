package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type AuthParsedTokenContext string

const authParsedTokenContextKey AuthParsedTokenContext = "auth_token_claims_ctx"

func parseIncomingToken(ctx context.Context, oidcIssuer, oidcClientID *string, k jwt.Keyfunc) (*CustomClaims, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, errors.New("GRPC metadata could not be extracted from incoming context")
	}
	authHeaders := md.Get("Authorization")
	if len(authHeaders) != 1 {
		return nil, fmt.Errorf("authorization header appeared %d times, not exactly once", len(authHeaders))
	}
	if !strings.HasPrefix(authHeaders[0], "Bearer ") {
		return nil, errors.New("authorization Header was not in bearer format")
	}

	authToken := strings.TrimPrefix(authHeaders[0], "Bearer ")
	claims := CustomClaims{}

	parsed, err := jwt.ParseWithClaims(
		authToken,
		&claims,
		k,
		jwt.WithIssuer(*oidcIssuer),
		jwt.WithAudience(*oidcClientID),
		jwt.WithIssuedAt(),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to parse and verify token: %v", err)
	}
	if !parsed.Valid {
		return nil, errors.New("parsed token was not valid")
	}
	claims.ClientID = oidcClientID
	return &claims, nil
}

func GetGrpcAuthInterceptor(oidcIssuer, oidcClientID *string, unauthenticated *bool, k jwt.Keyfunc) grpc.UnaryServerInterceptor {
	logger := logrus.WithFields(logrus.Fields{
		"component": "grpc-auth-interceptor",
	})

	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		claims, err := parseIncomingToken(ctx, oidcIssuer, oidcClientID, k)
		if err != nil {
			if *unauthenticated {
				logger.Debugf("Allowing missing or corrupt token as we are in unauthenticated mode...")
			} else {
				logger.Errorf("could not get token from context: %v", err)
				return nil, status.Error(codes.Unauthenticated, "Token invalid or missing")
			}
		}

		enrichedCtx := context.WithValue(ctx, authParsedTokenContextKey, claims)
		res, err := handler(enrichedCtx, req)
		if err != nil {
			logger.Warnf("request %v for %v failed: %v", req, info, err)
		}
		return res, err
	}
}

func GetClaimsFromEnrichedGrpcCtx(ctx context.Context) (*CustomClaims, error) {
	authClaims := ctx.Value(authParsedTokenContextKey)
	if authClaims == nil {
		return nil, status.Errorf(codes.Unauthenticated, "auth token incorrect & missing")
	}
	var claims *CustomClaims
	var ok bool
	claims, ok = authClaims.(*CustomClaims)
	if !ok {
		return nil, status.Errorf(codes.Unauthenticated, "auth token claims of invalid type in enriched context - %+v", authClaims)
	}
	return claims, nil
}

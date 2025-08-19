package walserver

import (
	"context"
	"encoding/base64"
	"strings"

	"github.com/tg123/go-htpasswd"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

var (
	errMissingMetadata            = status.Errorf(codes.InvalidArgument, "missing metadata")
	errMissingAuthorizationHeader = status.Errorf(codes.InvalidArgument, "missing authorization header")
	errMissingAuthorizationValue  = status.Errorf(codes.InvalidArgument, "missing authorization value")
	errWrongAuthorizationValue    = status.Errorf(codes.InvalidArgument, "wrong authorization value")
	errInitializingHtpasswd       = status.Errorf(codes.InvalidArgument, "initializing htpasswd")
	errInvalidCredentials         = status.Errorf(codes.Unauthenticated, "invalid credentials")
)

// parseBasicAuth parses a basic auth header and comes directly from the
// http go package, where it is declared private.
//
//nolint:nonamedreturns
func parseBasicAuth(auth string) (username, password string, ok bool) {
	const prefix = "Basic "
	// Case insensitive prefix match. See Issue 22736.
	if len(auth) < len(prefix) || !strings.EqualFold(auth[:len(prefix)], prefix) {
		return "", "", false
	}
	c, err := base64.StdEncoding.DecodeString(auth[len(prefix):])
	if err != nil {
		return "", "", false
	}
	cs := string(c)
	username, password, ok = strings.Cut(cs, ":")
	if !ok {
		return "", "", false
	}

	return username, password, true
}

// EnsureValidAuthentication ensures that the credentials are valid checking them
// with the passed htpasswd file.
func EnsureValidAuthentication(htpasswdFile string) (grpc.UnaryServerInterceptor, error) {
	auth, err := htpasswd.New(htpasswdFile, htpasswd.DefaultSystems, nil)
	if err != nil {
		return nil, errInitializingHtpasswd
	}

	//nolint:nonamedreturns
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any,
		err error,
	) {
		md, hasMetadata := metadata.FromIncomingContext(ctx)
		if !hasMetadata {
			return nil, errMissingMetadata
		}

		authHeader, hasAuthHeader := md["authorization"]
		if !hasAuthHeader {
			return nil, errMissingAuthorizationHeader
		}

		if len(authHeader) == 0 {
			return nil, errMissingAuthorizationValue
		}

		username, password, ok := parseBasicAuth(authHeader[0])
		if !ok {
			return nil, errWrongAuthorizationValue
		}

		if !auth.Match(username, password) {
			return nil, errInvalidCredentials
		}

		return handler(ctx, req)
	}, nil
}

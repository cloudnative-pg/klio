package grpcclient

import (
	"context"
	"encoding/base64"
	"fmt"
)

type basicAuthCredentials struct {
	username string
	password string
}

// GetRequestMetadata implement the grpc.UnaryInterceptor interface.
func (b *basicAuthCredentials) GetRequestMetadata(_ context.Context, _ ...string) (map[string]string, error) {
	authData := base64.StdEncoding.EncodeToString(fmt.Appendf(nil, "%s:%s", b.username, b.password))
	return map[string]string{"authorization": "Basic " + authData}, nil
}

// RequireTransportSecurity implement the grpc.UnaryInterceptor interface.
func (b *basicAuthCredentials) RequireTransportSecurity() bool {
	// this requires TLS
	return true
}

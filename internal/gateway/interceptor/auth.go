// Package interceptor provides cross-cutting gRPC server interceptors:
// bearer-token authentication, panic recovery and a concurrency limit. They are
// plain functions, so they can be unit-tested without a network connection.
package interceptor

import (
	"context"
	"crypto/subtle"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	authMetadataKey = "authorization"
	bearerPrefix    = "Bearer "
)

// StreamAuth returns a stream interceptor that requires a valid bearer token in
// the request's "authorization" metadata.
func StreamAuth(token string) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if err := authorize(ss.Context(), token); err != nil {
			return err
		}
		return handler(srv, ss)
	}
}

// UnaryAuth is the unary counterpart of StreamAuth.
func UnaryAuth(token string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if err := authorize(ctx, token); err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

// authorize checks the incoming metadata for a valid "Bearer <token>" header,
// comparing in constant time so the token cannot be guessed via timing.
func authorize(ctx context.Context, want string) error {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return status.Error(codes.Unauthenticated, "missing request metadata")
	}
	values := md.Get(authMetadataKey)
	if len(values) == 0 {
		return status.Error(codes.Unauthenticated, "missing authorization header")
	}
	header := values[0]
	if !strings.HasPrefix(header, bearerPrefix) {
		return status.Error(codes.Unauthenticated, "authorization header must be a bearer token")
	}
	got := strings.TrimPrefix(header, bearerPrefix)
	if subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
		return status.Error(codes.Unauthenticated, "invalid authorization token")
	}
	return nil
}

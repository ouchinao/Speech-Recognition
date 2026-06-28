package interceptor

import (
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ConcurrencyLimitStream returns a stream interceptor that allows at most limit
// concurrent streams, rejecting further ones with ResourceExhausted. This
// bounds the number of live recognizers (and thus memory) the server holds.
//
// A limit of zero or less disables the limit (every stream passes through),
// rather than panicking or rejecting everything.
func ConcurrencyLimitStream(limit int) grpc.StreamServerInterceptor {
	if limit <= 0 {
		return func(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			return handler(srv, ss)
		}
	}

	slots := make(chan struct{}, limit)
	return func(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
			return handler(srv, ss)
		default:
			return status.Error(codes.ResourceExhausted, "too many concurrent streams")
		}
	}
}

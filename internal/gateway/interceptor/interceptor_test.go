package interceptor

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// fakeStream is a grpc.ServerStream whose only real method is Context().
type fakeStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (f fakeStream) Context() context.Context { return f.ctx }

var testInfo = &grpc.StreamServerInfo{FullMethod: "/speech.v1.SpeechRecognition/Recognize"}

func okHandler(any, grpc.ServerStream) error { return nil }

func authCtx(value string) context.Context {
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", value))
}

func TestStreamAuth(t *testing.T) {
	const secret = "s3cr3t"
	intc := StreamAuth(secret)

	tests := []struct {
		name string
		ctx  context.Context
		want codes.Code
	}{
		{"valid bearer token", authCtx("Bearer " + secret), codes.OK},
		{"no metadata at all", context.Background(), codes.Unauthenticated},
		{"empty metadata", metadata.NewIncomingContext(context.Background(), metadata.MD{}), codes.Unauthenticated},
		{"wrong token", authCtx("Bearer nope"), codes.Unauthenticated},
		{"missing bearer prefix", authCtx(secret), codes.Unauthenticated},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			handler := func(any, grpc.ServerStream) error { called = true; return nil }

			err := intc(nil, fakeStream{ctx: tt.ctx}, testInfo, handler)
			if status.Code(err) != tt.want {
				t.Fatalf("code = %v, want %v", status.Code(err), tt.want)
			}
			if wantCalled := tt.want == codes.OK; called != wantCalled {
				t.Errorf("handler called = %v, want %v", called, wantCalled)
			}
		})
	}
}

func TestUnaryAuth(t *testing.T) {
	const secret = "s3cr3t"
	intc := UnaryAuth(secret)
	info := &grpc.UnaryServerInfo{}
	handler := func(context.Context, any) (any, error) { return "ok", nil }

	if _, err := intc(authCtx("Bearer "+secret), nil, info, handler); err != nil {
		t.Errorf("valid token rejected: %v", err)
	}
	if _, err := intc(context.Background(), nil, info, handler); status.Code(err) != codes.Unauthenticated {
		t.Errorf("code = %v, want Unauthenticated", status.Code(err))
	}
}

func TestRecoveryStream(t *testing.T) {
	intc := RecoveryStream()
	panicking := func(any, grpc.ServerStream) error { panic("boom") }

	err := intc(nil, fakeStream{ctx: context.Background()}, testInfo, panicking)
	if status.Code(err) != codes.Internal {
		t.Fatalf("code = %v, want Internal (panic recovered)", status.Code(err))
	}
}

func TestConcurrencyLimitStreamDisabled(t *testing.T) {
	// 0 or negative must disable the limit (no panic, no rejection), not crash
	// the server or reject every stream.
	for _, limit := range []int{0, -1} {
		intc := ConcurrencyLimitStream(limit) // must not panic for -1
		called := false
		err := intc(nil, fakeStream{ctx: context.Background()}, testInfo, func(any, grpc.ServerStream) error {
			called = true
			return nil
		})
		if err != nil {
			t.Errorf("limit=%d: err = %v, want nil (limit disabled)", limit, err)
		}
		if !called {
			t.Errorf("limit=%d: handler not called", limit)
		}
	}
}

func TestConcurrencyLimitStream(t *testing.T) {
	intc := ConcurrencyLimitStream(1)

	started := make(chan struct{})
	release := make(chan struct{})
	blocking := func(any, grpc.ServerStream) error {
		close(started)
		<-release
		return nil
	}

	go func() { _ = intc(nil, fakeStream{ctx: context.Background()}, testInfo, blocking) }()
	<-started // the first stream now holds the only slot

	err := intc(nil, fakeStream{ctx: context.Background()}, testInfo, okHandler)
	if status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("second stream code = %v, want ResourceExhausted", status.Code(err))
	}

	close(release)
}

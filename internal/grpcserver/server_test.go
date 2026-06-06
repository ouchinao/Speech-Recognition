package grpcserver

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"

	"speech-recognition/internal/genproto/speechv1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

// fakeRecognizer scripts partial results per frame and a flushed final result.
type fakeRecognizer struct {
	partial string
	flush   string
	closed  bool
}

func (r *fakeRecognizer) AcceptWaveform(frame []byte) bool { return false }
func (r *fakeRecognizer) Result() string                   { return "" }
func (r *fakeRecognizer) PartialResult() string            { return r.partial }
func (r *fakeRecognizer) FinalResult() string              { return r.flush }
func (r *fakeRecognizer) Close() error                     { r.closed = true; return nil }

type fakeEngine struct {
	rec        *fakeRecognizer
	sampleRate int
}

func (e *fakeEngine) NewRecognizer(sampleRate int) (StreamRecognizer, error) {
	e.sampleRate = sampleRate
	return e.rec, nil
}

// dialServer starts the Server on an in-memory listener and returns a connected
// client plus the engine, for assertions.
func dialServer(t *testing.T, defaultSampleRate int) (speechv1.SpeechRecognitionClient, *fakeEngine) {
	t.Helper()

	engine := &fakeEngine{rec: &fakeRecognizer{partial: "he", flush: "hello"}}
	lis := bufconn.Listen(1024 * 1024)
	srv := grpc.NewServer()
	speechv1.RegisterSpeechRecognitionServer(srv, New(engine, defaultSampleRate))
	go func() { _ = srv.Serve(lis) }()

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
		srv.Stop()
	})
	return speechv1.NewSpeechRecognitionClient(conn), engine
}

func TestRecognizeStreamsPartialsAndFinal(t *testing.T) {
	client, engine := dialServer(t, 16000)

	stream, err := client.Recognize(context.Background())
	if err != nil {
		t.Fatalf("Recognize: %v", err)
	}

	// Config first (request a non-default sample rate), then two audio frames.
	mustSend(t, stream, &speechv1.RecognizeRequest{
		Request: &speechv1.RecognizeRequest_Config{
			Config: &speechv1.RecognitionConfig{SampleRateHertz: 8000},
		},
	})
	mustSend(t, stream, audioReq([]byte{1, 2}))
	mustSend(t, stream, audioReq([]byte{3, 4}))
	if err := stream.CloseSend(); err != nil {
		t.Fatalf("CloseSend: %v", err)
	}

	var got []*speechv1.RecognizeResponse
	for {
		resp, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("Recv: %v", err)
		}
		got = append(got, resp)
	}

	// Two partials (one per frame) followed by the flushed final.
	if len(got) != 3 {
		t.Fatalf("got %d responses, want 3: %+v", len(got), got)
	}
	if got[0].GetText() != "he" || got[0].GetIsFinal() {
		t.Errorf("response[0] = %+v, want partial 'he'", got[0])
	}
	if got[2].GetText() != "hello" || !got[2].GetIsFinal() {
		t.Errorf("response[2] = %+v, want final 'hello'", got[2])
	}
	if engine.sampleRate != 8000 {
		t.Errorf("engine sample rate = %d, want 8000 from config", engine.sampleRate)
	}
	if !engine.rec.closed {
		t.Error("recognizer was not closed at end of stream")
	}
}

func TestRecognizeRejectsMissingConfig(t *testing.T) {
	client, _ := dialServer(t, 16000)

	stream, err := client.Recognize(context.Background())
	if err != nil {
		t.Fatalf("Recognize: %v", err)
	}
	// Send audio before any config: the server must reject the stream.
	mustSend(t, stream, audioReq([]byte{1, 2}))
	if err := stream.CloseSend(); err != nil {
		t.Fatalf("CloseSend: %v", err)
	}

	_, err = stream.Recv()
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("Recv error = %v (code %v), want InvalidArgument for missing config", err, status.Code(err))
	}
}

func audioReq(b []byte) *speechv1.RecognizeRequest {
	return &speechv1.RecognizeRequest{
		Request: &speechv1.RecognizeRequest_AudioContent{AudioContent: b},
	}
}

func mustSend(t *testing.T, stream grpc.BidiStreamingClient[speechv1.RecognizeRequest, speechv1.RecognizeResponse], req *speechv1.RecognizeRequest) {
	t.Helper()
	if err := stream.Send(req); err != nil {
		t.Fatalf("Send: %v", err)
	}
}

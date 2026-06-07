package grpcserver

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

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

// fakeMic is an injected microphone that hands out a fixed frame indefinitely.
type fakeMic struct {
	frame  []byte
	opened int
	closed int
}

func (m *fakeMic) Open(sampleRate int) (MicStream, error) {
	m.opened++
	return &fakeMicStream{frame: m.frame, mic: m}, nil
}

type fakeMicStream struct {
	frame []byte
	mic   *fakeMic
}

func (s *fakeMicStream) Read() ([]byte, error) { return s.frame, nil }
func (s *fakeMicStream) Close() error          { s.mic.closed++; return nil }

// loudFrame returns a PCM frame whose RMS is well above the VAD threshold, so a
// real vad detector classifies it as speech.
func loudFrame() []byte {
	const n = 64
	b := make([]byte, n*2)
	for i := 0; i < n; i++ {
		binary.LittleEndian.PutUint16(b[i*2:], uint16(int16(16384)))
	}
	return b
}

// dialServer starts the Server on an in-memory listener and returns a connected
// client plus the engine, for assertions.
func dialServer(t *testing.T, defaultSampleRate int) (speechv1.SpeechRecognitionClient, *fakeEngine) {
	t.Helper()
	engine := &fakeEngine{rec: &fakeRecognizer{partial: "he", flush: "hello"}}
	return newTestClient(t, engine, nil, defaultSampleRate, 0), engine
}

// newTestClient starts a Server with the given collaborators over bufconn.
func newTestClient(t *testing.T, engine Engine, mic Microphone, defaultSampleRate int, calibration time.Duration) speechv1.SpeechRecognitionClient {
	t.Helper()

	lis := bufconn.Listen(1024 * 1024)
	srv := grpc.NewServer()
	speechv1.RegisterSpeechRecognitionServer(srv, New(engine, mic, defaultSampleRate, calibration))
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
	return speechv1.NewSpeechRecognitionClient(conn)
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

func TestRecognizeMicrophoneStreamsTranscripts(t *testing.T) {
	mic := &fakeMic{frame: loudFrame()}
	engine := &fakeEngine{rec: &fakeRecognizer{partial: "he"}}
	// calibration 0 keeps the VAD at its lenient initial threshold so the loud
	// frames register as speech immediately.
	client := newTestClient(t, engine, mic, 16000, 0)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream, err := client.RecognizeMicrophone(ctx, &speechv1.RecognizeMicrophoneRequest{VadMode: 0})
	if err != nil {
		t.Fatalf("RecognizeMicrophone: %v", err)
	}

	resp, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv: %v", err)
	}
	if resp.GetText() != "he" || resp.GetIsFinal() {
		t.Errorf("response = %+v, want partial 'he' from the server microphone", resp)
	}
	if mic.opened != 1 {
		t.Errorf("mic opened %d times, want 1", mic.opened)
	}

	cancel()
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

func TestRecognizeStatus(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name string
		ctx  context.Context
		err  error
		want codes.Code
	}{
		{"nil error", context.Background(), nil, codes.OK},
		{"cancelled context", cancelled, fmt.Errorf("read audio: %w", status.Error(codes.Canceled, "x")), codes.Canceled},
		{"too many non-audio", context.Background(), fmt.Errorf("read audio: %w", errTooManyNonAudio), codes.InvalidArgument},
		{"context.Canceled sentinel", context.Background(), fmt.Errorf("read audio: %w", context.Canceled), codes.Canceled},
		{"generic error", context.Background(), errors.New("boom"), codes.Internal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := status.Code(recognizeStatus(tt.ctx, tt.err)); got != tt.want {
				t.Errorf("recognizeStatus code = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRecognizeRejectsTooManyNonAudio(t *testing.T) {
	old := maxConsecutiveNonAudio
	maxConsecutiveNonAudio = 3
	defer func() { maxConsecutiveNonAudio = old }()

	client, _ := dialServer(t, 16000)
	stream, err := client.Recognize(context.Background())
	if err != nil {
		t.Fatalf("Recognize: %v", err)
	}

	cfg := &speechv1.RecognizeRequest{
		Request: &speechv1.RecognizeRequest_Config{Config: &speechv1.RecognitionConfig{}},
	}
	mustSend(t, stream, cfg) // handshake config
	for i := 0; i < maxConsecutiveNonAudio+2; i++ {
		_ = stream.Send(cfg) // a flood of config-only (non-audio) messages
	}
	_ = stream.CloseSend()

	if _, err := stream.Recv(); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("Recv code = %v, want InvalidArgument", status.Code(err))
	}
}

func TestRecognizeMicrophoneRejectsConcurrent(t *testing.T) {
	mic := &fakeMic{frame: loudFrame()}
	engine := &fakeEngine{rec: &fakeRecognizer{partial: "he"}}
	client := newTestClient(t, engine, mic, 16000, 0)

	ctxA, cancelA := context.WithCancel(context.Background())
	defer cancelA()
	streamA, err := client.RecognizeMicrophone(ctxA, &speechv1.RecognizeMicrophoneRequest{})
	if err != nil {
		t.Fatalf("stream A: %v", err)
	}
	// Reading one response guarantees the server holds the microphone lock.
	if _, err := streamA.Recv(); err != nil {
		t.Fatalf("stream A Recv: %v", err)
	}

	streamB, err := client.RecognizeMicrophone(context.Background(), &speechv1.RecognizeMicrophoneRequest{})
	if err != nil {
		t.Fatalf("stream B: %v", err)
	}
	if _, err := streamB.Recv(); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("stream B code = %v, want FailedPrecondition", status.Code(err))
	}
	if mic.opened != 1 {
		t.Errorf("mic opened %d times, want 1 (second stream rejected before opening)", mic.opened)
	}
	cancelA()
}

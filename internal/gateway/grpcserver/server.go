// Package grpcserver adapts the recognition use case to the SpeechRecognition
// gRPC service. It owns the small interfaces it consumes (Engine,
// StreamRecognizer), so it depends on no cgo packages and is testable over an
// in-memory connection; the concrete VOSK engine is injected from the command.
package grpcserver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"speech-recognition/internal/gateway/genproto/speechv1"
	"speech-recognition/internal/usecase/recognition"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// errTooManyNonAudio is returned by streamSource when a client floods the
// stream with non-audio messages instead of sending audio.
var errTooManyNonAudio = errors.New("too many consecutive non-audio messages")

// maxConsecutiveNonAudio bounds how many non-audio messages streamSource will
// skip before failing, so an idle/abusive client cannot pin a recognizer
// forever. It is a var so tests can lower it.
var maxConsecutiveNonAudio = 1024

// StreamRecognizer is a per-stream speech-to-text engine.
type StreamRecognizer interface {
	recognition.Recognizer
	// Close releases the recognizer's resources at the end of a stream.
	Close() error
}

// Engine creates a StreamRecognizer for a sample rate. Implementations
// typically wrap a single, shared, expensive-to-load model.
type Engine interface {
	NewRecognizer(sampleRate int) (StreamRecognizer, error)
}

// Microphone opens the server's local microphone as an audio source. It is
// injected from the command so this package stays free of cgo.
type Microphone interface {
	Open(sampleRate int) (MicStream, error)
}

// MicStream is an open microphone: an audio source that can be closed.
type MicStream interface {
	recognition.AudioSource
	Close() error
}

// DetectorFactory builds a voice detector for a sample rate and VAD mode. The
// concrete domain detector is injected from the command, so this package never
// imports the domain package directly.
type DetectorFactory interface {
	NewDetector(sampleRate, mode int) recognition.VoiceDetector
}

// Server implements the SpeechRecognition gRPC service.
type Server struct {
	speechv1.UnimplementedSpeechRecognitionServer

	engine            Engine
	mic               Microphone
	detectors         DetectorFactory
	defaultSampleRate int
	calibration       time.Duration

	// micLock serializes access to the single physical microphone so only one
	// RecognizeMicrophone stream runs at a time.
	micLock sync.Mutex
}

// New returns a Server backed by engine. defaultSampleRate is used when a
// client omits one; mic and detectors (which may be nil to disable
// RecognizeMicrophone) supply the server's microphone and the voice detector
// built per stream, calibrated for the given duration.
func New(engine Engine, mic Microphone, detectors DetectorFactory, defaultSampleRate int, calibration time.Duration) *Server {
	return &Server{
		engine:            engine,
		mic:               mic,
		detectors:         detectors,
		defaultSampleRate: defaultSampleRate,
		calibration:       calibration,
	}
}

// Recognize handles a bidirectional recognition stream: it reads the leading
// configuration message, then streams transcripts back as audio arrives.
func (s *Server) Recognize(stream speechv1.SpeechRecognition_RecognizeServer) error {
	first, err := stream.Recv()
	if errors.Is(err, io.EOF) {
		return nil // client hung up before sending anything
	}
	if err != nil {
		return err
	}

	cfg := first.GetConfig()
	if cfg == nil {
		return status.Error(codes.InvalidArgument, "first message must be a recognition config")
	}

	sampleRate := int(cfg.GetSampleRateHertz())
	if sampleRate <= 0 {
		sampleRate = s.defaultSampleRate
	}

	rec, err := s.engine.NewRecognizer(sampleRate)
	if err != nil {
		return status.Errorf(codes.Internal, "create recognizer: %v", err)
	}
	defer func() { _ = rec.Close() }()

	source := &streamSource{stream: stream}
	printer := &streamPrinter{stream: stream}

	runErr := recognition.NewStreamer(source, rec, printer).Run(stream.Context())
	return recognizeStatus(stream.Context(), runErr)
}

// recognizeStatus maps the Streamer's result to a gRPC status. A cancelled
// stream context (the client disconnected) is reported as Canceled rather than
// Internal, and a non-audio flood as InvalidArgument.
func recognizeStatus(ctx context.Context, err error) error {
	switch {
	case err == nil:
		return nil
	case ctx.Err() != nil:
		return status.FromContextError(ctx.Err()).Err()
	case errors.Is(err, errTooManyNonAudio):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, context.Canceled):
		return status.Error(codes.Canceled, "stream cancelled")
	default:
		return status.Errorf(codes.Internal, "recognition: %v", err)
	}
}

// RecognizeMicrophone transcribes the server's local microphone and streams the
// results to the client until the client disconnects. It reuses the microphone
// use case (recognition.Service, with VAD and calibration) and merely swaps the
// console printer for a gRPC one.
func (s *Server) RecognizeMicrophone(req *speechv1.RecognizeMicrophoneRequest, stream speechv1.SpeechRecognition_RecognizeMicrophoneServer) error {
	if s.mic == nil {
		return status.Error(codes.Unimplemented, "server microphone is not configured")
	}
	// The microphone is a single physical device; reject overlapping streams.
	if !s.micLock.TryLock() {
		return status.Error(codes.FailedPrecondition, "microphone is already in use by another stream")
	}
	defer s.micLock.Unlock()

	ctx := stream.Context()

	micStream, err := s.mic.Open(s.defaultSampleRate)
	if err != nil {
		return status.Errorf(codes.Internal, "open microphone: %v", err)
	}
	defer func() { _ = micStream.Close() }()

	rec, err := s.engine.NewRecognizer(s.defaultSampleRate)
	if err != nil {
		return status.Errorf(codes.Internal, "create recognizer: %v", err)
	}
	defer func() { _ = rec.Close() }()

	detector := s.detectors.NewDetector(s.defaultSampleRate, int(req.GetVadMode()))
	printer := &streamPrinter{stream: stream}
	svc := recognition.NewService(micStream, detector, rec, printer)

	if err := svc.Calibrate(ctx, s.calibration); err != nil {
		return micStatus(err, "calibration")
	}
	// Service.Run only returns on cancellation or a fatal read error (never nil).
	return micStatus(svc.Run(ctx), "recognition")
}

// micStatus maps a use-case error to a gRPC status, treating cancellation as a
// clean client disconnect.
func micStatus(err error, stage string) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, context.Canceled):
		return status.Error(codes.Canceled, "stream cancelled")
	default:
		return status.Errorf(codes.Internal, "%s: %v", stage, err)
	}
}

// streamSource adapts the inbound gRPC stream to recognition.AudioSource.
type streamSource struct {
	stream   speechv1.SpeechRecognition_RecognizeServer
	nonAudio int
}

// Read returns the next audio frame and returns io.EOF when the client closes
// its half of the stream. Non-audio messages (a stray config) are skipped, but
// only up to maxConsecutiveNonAudio in a row to bound abusive/idle clients.
func (s *streamSource) Read() ([]byte, error) {
	for {
		msg, err := s.stream.Recv()
		if err != nil {
			return nil, err
		}
		if audio, ok := msg.GetRequest().(*speechv1.RecognizeRequest_AudioContent); ok {
			s.nonAudio = 0
			return audio.AudioContent, nil
		}
		s.nonAudio++
		if s.nonAudio > maxConsecutiveNonAudio {
			return nil, fmt.Errorf("%w (%d)", errTooManyNonAudio, s.nonAudio)
		}
		// A config (or empty) message mid-stream is ignored; read the next one.
	}
}

// responseSender is the outbound half shared by the bidirectional and
// server-streaming RPCs, so streamPrinter works with either.
type responseSender interface {
	Send(*speechv1.RecognizeResponse) error
}

// streamPrinter adapts recognition.Printer to the outbound gRPC stream.
type streamPrinter struct {
	stream responseSender
}

// Final sends a finalized transcript. Send errors are ignored: the next Recv
// will surface the broken stream and end recognition.
func (p *streamPrinter) Final(text string) {
	_ = p.stream.Send(&speechv1.RecognizeResponse{Text: text, IsFinal: true})
}

// Partial sends an in-progress transcript.
func (p *streamPrinter) Partial(text string) {
	_ = p.stream.Send(&speechv1.RecognizeResponse{Text: text, IsFinal: false})
}

// Status is a no-op: VAD telemetry is not exposed over the API.
func (p *streamPrinter) Status(rms, threshold float64) {}

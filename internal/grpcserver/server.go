// Package grpcserver adapts the recognition use case to the SpeechRecognition
// gRPC service. It owns the small interfaces it consumes (Engine,
// StreamRecognizer), so it depends on no cgo packages and is testable over an
// in-memory connection; the concrete VOSK engine is injected from the command.
package grpcserver

import (
	"context"
	"errors"
	"io"
	"time"

	"speech-recognition/internal/genproto/speechv1"
	"speech-recognition/internal/recognition"
	"speech-recognition/internal/vad"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

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

// Server implements the SpeechRecognition gRPC service.
type Server struct {
	speechv1.UnimplementedSpeechRecognitionServer

	engine            Engine
	mic               Microphone
	defaultSampleRate int
	calibration       time.Duration
}

// New returns a Server backed by engine. defaultSampleRate is used when a
// client omits one; mic (which may be nil to disable RecognizeMicrophone)
// supplies the server's microphone, calibrated for the given duration.
func New(engine Engine, mic Microphone, defaultSampleRate int, calibration time.Duration) *Server {
	return &Server{
		engine:            engine,
		mic:               mic,
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

	if err := recognition.NewStreamer(source, rec, printer).Run(stream.Context()); err != nil {
		if errors.Is(err, context.Canceled) {
			return status.Error(codes.Canceled, "stream cancelled")
		}
		return status.Errorf(codes.Internal, "recognition: %v", err)
	}
	return nil
}

// RecognizeMicrophone transcribes the server's local microphone and streams the
// results to the client until the client disconnects. It reuses the microphone
// use case (recognition.Service, with VAD and calibration) and merely swaps the
// console printer for a gRPC one.
func (s *Server) RecognizeMicrophone(req *speechv1.RecognizeMicrophoneRequest, stream speechv1.SpeechRecognition_RecognizeMicrophoneServer) error {
	if s.mic == nil {
		return status.Error(codes.Unimplemented, "server microphone is not configured")
	}
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

	detector := vad.New(s.defaultSampleRate, int(req.GetVadMode()))
	printer := &streamPrinter{stream: stream}
	svc := recognition.NewService(micStream, detector, rec, printer)

	if err := svc.Calibrate(ctx, s.calibration); err != nil {
		return micStatus(err, "calibration")
	}
	if err := svc.Run(ctx); err != nil {
		return micStatus(err, "recognition")
	}
	return nil
}

// micStatus maps a use-case error to a gRPC status, treating cancellation as a
// clean client disconnect.
func micStatus(err error, stage string) error {
	if errors.Is(err, context.Canceled) {
		return status.Error(codes.Canceled, "stream cancelled")
	}
	return status.Errorf(codes.Internal, "%s: %v", stage, err)
}

// streamSource adapts the inbound gRPC stream to recognition.AudioSource.
type streamSource struct {
	stream speechv1.SpeechRecognition_RecognizeServer
}

// Read returns the next audio frame, ignoring any further config messages, and
// returns io.EOF when the client closes its half of the stream.
func (s *streamSource) Read() ([]byte, error) {
	for {
		msg, err := s.stream.Recv()
		if err != nil {
			return nil, err
		}
		if audio, ok := msg.GetRequest().(*speechv1.RecognizeRequest_AudioContent); ok {
			return audio.AudioContent, nil
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

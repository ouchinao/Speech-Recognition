// Package grpcserver adapts the recognition use case to the SpeechRecognition
// gRPC service. It owns the small interfaces it consumes (Engine,
// StreamRecognizer), so it depends on no cgo packages and is testable over an
// in-memory connection; the concrete VOSK engine is injected from the command.
package grpcserver

import (
	"context"
	"errors"
	"io"

	"speech-recognition/internal/genproto/speechv1"
	"speech-recognition/internal/recognition"

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

// Server implements the SpeechRecognition gRPC service.
type Server struct {
	speechv1.UnimplementedSpeechRecognitionServer

	engine            Engine
	defaultSampleRate int
}

// New returns a Server backed by engine. defaultSampleRate is used when a
// client does not specify one in its configuration message.
func New(engine Engine, defaultSampleRate int) *Server {
	return &Server{engine: engine, defaultSampleRate: defaultSampleRate}
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

// streamPrinter adapts recognition.Printer to the outbound gRPC stream.
type streamPrinter struct {
	stream speechv1.SpeechRecognition_RecognizeServer
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

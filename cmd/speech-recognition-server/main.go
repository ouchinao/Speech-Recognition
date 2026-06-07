// Command speech-recognition-server exposes the VOSK speech recognition engine
// as a streaming gRPC service. It is the composition root for the server: it
// loads the model once, adapts it to the grpcserver.Engine interface and serves.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os/signal"
	"syscall"
	"time"

	"speech-recognition/internal/audio"
	"speech-recognition/internal/genproto/speechv1"
	"speech-recognition/internal/grpcserver"
	"speech-recognition/internal/recognizer"

	"google.golang.org/grpc"
)

// micFramesPerBuffer is the number of frames read per microphone capture call.
const micFramesPerBuffer = 1024

func main() {
	if err := run(); err != nil {
		log.Fatalf("speech-recognition-server: %v", err)
	}
}

func run() error {
	addr := flag.String("addr", ":50051", "gRPC listen address")
	modelPath := flag.String("models", "./models/vosk-model-ja-0.22", "Path to VOSK model")
	sampleRate := flag.Int("sample-rate", 16000, "Default sample rate (Hz) used when a client omits one")
	calibration := flag.Duration("calibration-time", 5*time.Second, "Ambient-noise calibration duration for RecognizeMicrophone")
	flag.Parse()

	model, err := recognizer.LoadModel(*modelPath)
	if err != nil {
		return fmt.Errorf("load model: %w", err)
	}
	defer func() { _ = model.Close() }()

	lis, err := net.Listen("tcp", *addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", *addr, err)
	}

	grpcServer := grpc.NewServer()
	server := grpcserver.New(voskEngine{model: model}, portAudioMic{framesPerBuffer: micFramesPerBuffer}, *sampleRate, *calibration)
	speechv1.RegisterSpeechRecognitionServer(grpcServer, server)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	serveErr := make(chan error, 1)
	go func() { serveErr <- grpcServer.Serve(lis) }()
	log.Printf("speech-recognition-server listening on %s (model %s)", *addr, *modelPath)

	select {
	case <-ctx.Done():
		log.Println("shutting down...")
		grpcServer.GracefulStop()
		return nil
	case err := <-serveErr:
		return fmt.Errorf("serve: %w", err)
	}
}

// voskEngine adapts a recognizer.Model to grpcserver.Engine, creating one VOSK
// recognizer per stream from the single shared model.
type voskEngine struct {
	model *recognizer.Model
}

func (e voskEngine) NewRecognizer(sampleRate int) (grpcserver.StreamRecognizer, error) {
	rec, err := e.model.NewRecognizer(sampleRate)
	if err != nil {
		return nil, err
	}
	return rec, nil
}

// portAudioMic adapts the PortAudio capture to grpcserver.Microphone, opening
// the default input device for RecognizeMicrophone.
type portAudioMic struct {
	framesPerBuffer int
}

func (m portAudioMic) Open(sampleRate int) (grpcserver.MicStream, error) {
	capture, err := audio.New(sampleRate, m.framesPerBuffer)
	if err != nil {
		return nil, err
	}
	return capture, nil
}

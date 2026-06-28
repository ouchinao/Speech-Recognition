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
	"os"
	"os/signal"
	"syscall"
	"time"

	"speech-recognition/internal/domain/vad"
	"speech-recognition/internal/gateway/genproto/speechv1"
	"speech-recognition/internal/gateway/grpcserver"
	"speech-recognition/internal/gateway/interceptor"
	"speech-recognition/internal/infrastructure/audio"
	"speech-recognition/internal/infrastructure/recognizer"
	"speech-recognition/internal/usecase/recognition"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
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
	tlsCert := flag.String("tls-cert", "", "Path to the TLS certificate (PEM); without -tls-key the server runs without TLS")
	tlsKey := flag.String("tls-key", "", "Path to the TLS private key (PEM)")
	authToken := flag.String("auth-token", os.Getenv("STT_AUTH_TOKEN"), "Bearer token clients must present (default $STT_AUTH_TOKEN); empty disables auth")
	maxStreams := flag.Int("max-streams", 64, "Maximum number of concurrent recognition streams")
	maxConnIdle := flag.Duration("max-conn-idle", 5*time.Minute, "Close a connection after it has been idle this long")
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

	opts, err := serverOptions(*tlsCert, *tlsKey, *authToken, *maxStreams, *maxConnIdle)
	if err != nil {
		return err
	}

	grpcServer := grpc.NewServer(opts...)
	server := grpcserver.New(
		voskEngine{model: model},
		portAudioMic{framesPerBuffer: micFramesPerBuffer},
		vadDetectorFactory{},
		*sampleRate,
		*calibration,
	)
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

// serverOptions assembles the gRPC server options: panic recovery, optional
// bearer-token auth, a concurrency limit, idle-connection reaping and optional
// TLS. Interceptors run outermost-first: recovery wraps auth wraps the limit.
func serverOptions(tlsCert, tlsKey, authToken string, maxStreams int, maxConnIdle time.Duration) ([]grpc.ServerOption, error) {
	streamInts := []grpc.StreamServerInterceptor{interceptor.RecoveryStream()}
	unaryInts := []grpc.UnaryServerInterceptor{interceptor.RecoveryUnary()}

	if authToken != "" {
		streamInts = append(streamInts, interceptor.StreamAuth(authToken))
		unaryInts = append(unaryInts, interceptor.UnaryAuth(authToken))
	} else {
		log.Println("WARNING: authentication is disabled (set -auth-token or $STT_AUTH_TOKEN)")
	}
	streamInts = append(streamInts, interceptor.ConcurrencyLimitStream(maxStreams))

	opts := []grpc.ServerOption{
		grpc.ChainStreamInterceptor(streamInts...),
		grpc.ChainUnaryInterceptor(unaryInts...),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle: maxConnIdle,
			Time:              1 * time.Minute,
			Timeout:           20 * time.Second,
		}),
	}

	switch {
	case tlsCert != "" && tlsKey != "":
		creds, err := credentials.NewServerTLSFromFile(tlsCert, tlsKey)
		if err != nil {
			return nil, fmt.Errorf("load TLS keypair: %w", err)
		}
		opts = append(opts, grpc.Creds(creds))
	case tlsCert != "" || tlsKey != "":
		return nil, fmt.Errorf("both -tls-cert and -tls-key are required to enable TLS")
	default:
		log.Println("WARNING: serving without TLS (set -tls-cert and -tls-key)")
	}

	return opts, nil
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

// vadDetectorFactory adapts the domain VAD constructor to
// grpcserver.DetectorFactory, so the gateway never imports the domain package.
type vadDetectorFactory struct{}

func (vadDetectorFactory) NewDetector(sampleRate, mode int) recognition.VoiceDetector {
	return vad.New(sampleRate, mode)
}

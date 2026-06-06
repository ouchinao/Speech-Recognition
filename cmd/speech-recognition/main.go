// Command speech-recognition is the entrypoint of the real-time VOSK speech
// recognition tool. It acts as the composition root: it loads configuration,
// constructs the concrete drivers and wires them into the recognition use case.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"speech-recognition/internal/audio"
	"speech-recognition/internal/config"
	"speech-recognition/internal/output"
	"speech-recognition/internal/recognition"
	"speech-recognition/internal/recognizer"
	"speech-recognition/internal/vad"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("speech-recognition: %v", err)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	fmt.Println("VOSK real-time speech recognition starting...")
	fmt.Printf("Loading model: %s\n", cfg.ModelPath)

	rec, err := recognizer.New(cfg.ModelPath, cfg.SampleRate)
	if err != nil {
		return fmt.Errorf("init recognizer: %w", err)
	}
	defer rec.Close()
	fmt.Println("Model loaded successfully")

	capture, err := audio.New(cfg.SampleRate, cfg.FramesPerBuffer)
	if err != nil {
		return fmt.Errorf("init audio capture: %w", err)
	}
	defer capture.Close()

	detector := vad.New(cfg.SampleRate, cfg.VADMode)
	printer := output.NewConsole(os.Stdout)
	service := recognition.NewService(capture, detector, rec, printer)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	fmt.Println("Microphone device: default input")
	fmt.Printf("Calibrating... (%v)\n", cfg.CalibrationDuration)
	if err := service.Calibrate(ctx, cfg.CalibrationDuration); err != nil {
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return fmt.Errorf("calibration: %w", err)
	}

	fmt.Printf("\nAmbient noise level: %.3f (threshold: %.3f)\n", detector.NoiseLevel(), detector.Threshold())
	fmt.Println("Speech recognition started")

	if err := service.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}

	fmt.Println("\n\nShutting down...")
	return nil
}

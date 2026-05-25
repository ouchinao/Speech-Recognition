package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"speech-recognition/audio"
	"speech-recognition/vad"
	"syscall"
	"time"

	vosk "github.com/alphacep/vosk-api/go"
)

const (
	sampleRate      = 16000
	framesPerBuffer = 1024
	calibrationTime = 5 * time.Second
)

func main() {
	modelPath := flag.String("models", "./models/vosk-model-ja-0.22", "Path to VOSK model")
	vadMode := flag.Int("vad-mode", 1, "VAD sensitivity (0-3: 0=lenient, 3=strict)")
	calibrationDuration := flag.Duration("calibration-time", calibrationTime, "Calibration duration")
	flag.Parse()

	fmt.Println("VOSK real-time speech recognition starting...")

	fmt.Printf("Loading model: %s\n", *modelPath)
	model, err := vosk.NewModel(*modelPath)
	if err != nil {
		log.Fatalf("Failed to load model: %v", err)
	}
	defer model.Free()
	fmt.Println("Model loaded successfully")

	recognizer, err := vosk.NewRecognizer(model, float64(sampleRate))
	if err != nil {
		log.Fatalf("Failed to create recognizer: %v", err)
	}
	defer recognizer.Free()

	recognizer.SetWords(1)

	vadDetector := vad.NewAdaptiveVAD(sampleRate, *vadMode)

	capture, err := audio.NewCapture(sampleRate, framesPerBuffer)
	if err != nil {
		log.Fatalf("Audio capture error: %v", err)
	}
	defer capture.Close()

	fmt.Println("Microphone device: default input")
	fmt.Printf("Calibrating... (%v)\n", *calibrationDuration)

	if err := calibrate(capture, vadDetector, *calibrationDuration); err != nil {
		log.Fatalf("Calibration error: %v", err)
	}

	threshold := vadDetector.GetThreshold()
	fmt.Printf("\nAmbient noise level: %.3f (threshold: %.3f)\n", vadDetector.GetNoiseLevel(), threshold)
	fmt.Println("Speech recognition started\n")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go recognitionLoop(capture, vadDetector, recognizer)

	<-sigChan
	fmt.Println("\n\nShutting down...")
}

func calibrate(capture *audio.Capture, vadDetector *vad.AdaptiveVAD, duration time.Duration) error {
	start := time.Now()
	for time.Since(start) < duration {
		buffer, err := capture.Read()
		if err != nil {
			return fmt.Errorf("calibration read error: %w", err)
		}
		vadDetector.Calibrate(buffer)
		time.Sleep(50 * time.Millisecond)
	}
	return nil
}

func recognitionLoop(capture *audio.Capture, vadDetector *vad.AdaptiveVAD, recognizer *vosk.VoskRecognizer) {
	for {
		buffer, err := capture.Read()
		if err != nil {
			log.Printf("Audio read error: %v", err)
			continue
		}

		rms := vadDetector.CalculateRMS(buffer)

		isSpeech := vadDetector.IsSpeech(buffer)

		if isSpeech {
			result := recognizer.AcceptWaveform(buffer)

			if result == 1 {
				finalResult := recognizer.Result()
				if len(finalResult) > 0 {
					fmt.Printf("\r\033[K[final] %s\n", extractText(finalResult))
				}
			} else {
				partialResult := recognizer.PartialResult()
				if len(partialResult) > 0 {
					fmt.Printf("\r[partial] %s", extractText(partialResult))
				}
			}

			fmt.Printf("\rRMS: %.3f | threshold: %.3f | speech detected", rms, vadDetector.GetThreshold())
		} else {
			vadDetector.UpdateThreshold(buffer)
		}
	}
}

func extractText(jsonResult string) string {
	var result struct {
		Text    string `json:"text"`
		Partial string `json:"partial"`
	}
	if err := json.Unmarshal([]byte(jsonResult), &result); err != nil {
		return ""
	}
	if result.Text != "" {
		return result.Text
	}
	if result.Partial != "" {
		return result.Partial
	}
	return ""
}

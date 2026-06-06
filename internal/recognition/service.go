// Package recognition contains the application use case that orchestrates the
// speech recognition pipeline. It depends only on the small interfaces declared
// here, so concrete drivers (PortAudio, VOSK, the console) are injected from
// the composition root and can be replaced with fakes in tests.
package recognition

import (
	"context"
	"fmt"
	"log"
	"time"
)

// calibrationPollInterval throttles reads while sampling ambient noise so that
// calibration spans real time rather than spinning as fast as the device.
const calibrationPollInterval = 50 * time.Millisecond

// AudioSource yields raw little-endian 16-bit PCM frames from an input device.
type AudioSource interface {
	Read() ([]byte, error)
}

// VoiceDetector classifies frames as speech or silence and adapts to noise.
type VoiceDetector interface {
	Calibrate(frame []byte)
	IsSpeech(frame []byte) bool
	CalculateRMS(frame []byte) float64
	UpdateThreshold(frame []byte)
	Threshold() float64
	NoiseLevel() float64
}

// Recognizer converts PCM frames into recognised text.
type Recognizer interface {
	AcceptWaveform(frame []byte) (complete bool)
	Result() string
	PartialResult() string
	// FinalResult flushes any buffered audio and returns its final text. It is
	// called once when the audio stream ends.
	FinalResult() string
}

// Printer renders recognition output to the user.
type Printer interface {
	Final(text string)
	Partial(text string)
	Status(rms, threshold float64)
}

// Service drives calibration and the recognition loop over its collaborators.
type Service struct {
	source     AudioSource
	detector   VoiceDetector
	recognizer Recognizer
	printer    Printer
}

// NewService wires the use case with its collaborators.
func NewService(source AudioSource, detector VoiceDetector, recognizer Recognizer, printer Printer) *Service {
	return &Service{
		source:     source,
		detector:   detector,
		recognizer: recognizer,
		printer:    printer,
	}
}

// Calibrate samples ambient noise for the given duration to establish the
// speech-detection threshold. It returns early if ctx is cancelled.
func (s *Service) Calibrate(ctx context.Context, duration time.Duration) error {
	deadline := time.Now().Add(duration)
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return err
		}

		frame, err := s.source.Read()
		if err != nil {
			return fmt.Errorf("read during calibration: %w", err)
		}
		s.detector.Calibrate(frame)

		time.Sleep(calibrationPollInterval)
	}
	return nil
}

// Run executes the recognition loop until ctx is cancelled, at which point it
// returns ctx.Err(). Transient read errors are logged and skipped.
func (s *Service) Run(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		frame, err := s.source.Read()
		if err != nil {
			log.Printf("audio read error: %v", err)
			continue
		}

		if !s.detector.IsSpeech(frame) {
			s.detector.UpdateThreshold(frame)
			continue
		}

		s.handleSpeech(frame)
	}
}

// handleSpeech feeds a speech frame to the recognizer and renders the result.
func (s *Service) handleSpeech(frame []byte) {
	if s.recognizer.AcceptWaveform(frame) {
		if text := s.recognizer.Result(); text != "" {
			s.printer.Final(text)
		}
	} else if text := s.recognizer.PartialResult(); text != "" {
		s.printer.Partial(text)
	}

	s.printer.Status(s.detector.CalculateRMS(frame), s.detector.Threshold())
}

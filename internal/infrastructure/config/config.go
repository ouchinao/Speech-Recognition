// Package config defines the runtime configuration for the application and
// the logic to load it from command-line flags.
package config

import (
	"flag"
	"os"
	"time"
)

// Default configuration values.
const (
	defaultModelPath       = "./models/vosk-model-ja-0.22"
	defaultVADMode         = 1
	defaultSampleRate      = 16000
	defaultFramesPerBuffer = 1024
	defaultCalibrationTime = 5 * time.Second
)

// Config holds every tunable parameter required to run the speech recognition
// pipeline. It is populated once at start-up and then treated as immutable.
type Config struct {
	// ModelPath is the filesystem path to the VOSK acoustic model.
	ModelPath string
	// VADMode selects the voice-activity-detection sensitivity (0=lenient .. 3=strict).
	VADMode int
	// CalibrationDuration is how long ambient noise is sampled before recognition starts.
	CalibrationDuration time.Duration
	// SampleRate is the audio sampling rate in Hz.
	SampleRate int
	// FramesPerBuffer is the number of audio frames read per capture call.
	FramesPerBuffer int
}

// Load parses the process command-line arguments and returns the resulting
// Config. It is a thin convenience wrapper around Parse.
func Load() (*Config, error) {
	return Parse(os.Args[0], os.Args[1:])
}

// Parse builds a Config from the provided arguments using an isolated FlagSet,
// which keeps the function free of global state and therefore unit-testable.
func Parse(name string, args []string) (*Config, error) {
	cfg := &Config{
		SampleRate:      defaultSampleRate,
		FramesPerBuffer: defaultFramesPerBuffer,
	}

	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.StringVar(&cfg.ModelPath, "models", defaultModelPath, "Path to VOSK model")
	fs.IntVar(&cfg.VADMode, "vad-mode", defaultVADMode, "VAD sensitivity (0-3: 0=lenient, 3=strict)")
	fs.DurationVar(&cfg.CalibrationDuration, "calibration-time", defaultCalibrationTime, "Calibration duration")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	return cfg, nil
}

package config

import (
	"testing"
	"time"
)

func TestParseDefaults(t *testing.T) {
	cfg, err := Parse("test", nil)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if cfg.ModelPath != defaultModelPath {
		t.Errorf("ModelPath = %q, want %q", cfg.ModelPath, defaultModelPath)
	}
	if cfg.VADMode != defaultVADMode {
		t.Errorf("VADMode = %d, want %d", cfg.VADMode, defaultVADMode)
	}
	if cfg.CalibrationDuration != defaultCalibrationTime {
		t.Errorf("CalibrationDuration = %v, want %v", cfg.CalibrationDuration, defaultCalibrationTime)
	}
	if cfg.SampleRate != defaultSampleRate {
		t.Errorf("SampleRate = %d, want %d", cfg.SampleRate, defaultSampleRate)
	}
	if cfg.FramesPerBuffer != defaultFramesPerBuffer {
		t.Errorf("FramesPerBuffer = %d, want %d", cfg.FramesPerBuffer, defaultFramesPerBuffer)
	}
}

func TestParseOverrides(t *testing.T) {
	args := []string{
		"-models", "/tmp/model",
		"-vad-mode", "3",
		"-calibration-time", "2s",
	}

	cfg, err := Parse("test", args)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if cfg.ModelPath != "/tmp/model" {
		t.Errorf("ModelPath = %q, want %q", cfg.ModelPath, "/tmp/model")
	}
	if cfg.VADMode != 3 {
		t.Errorf("VADMode = %d, want 3", cfg.VADMode)
	}
	if cfg.CalibrationDuration != 2*time.Second {
		t.Errorf("CalibrationDuration = %v, want %v", cfg.CalibrationDuration, 2*time.Second)
	}
}

func TestParseInvalidFlag(t *testing.T) {
	if _, err := Parse("test", []string{"-unknown"}); err == nil {
		t.Fatal("Parse(-unknown) = nil error, want error")
	}
}

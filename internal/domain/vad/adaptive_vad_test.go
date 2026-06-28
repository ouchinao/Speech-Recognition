package vad

import (
	"encoding/binary"
	"math"
	"testing"
)

// frameOf builds a little-endian 16-bit PCM frame of n samples each set to value.
func frameOf(value int16, n int) []byte {
	frame := make([]byte, n*2)
	for i := 0; i < n; i++ {
		binary.LittleEndian.PutUint16(frame[i*2:], uint16(value))
	}
	return frame
}

func TestCalculateRMS(t *testing.T) {
	tests := []struct {
		name  string
		value int16
		want  float64
	}{
		{"silence", 0, 0},
		{"half scale", 16384, 0.5},
		{"full scale", 32767, 32767.0 / 32768.0},
	}

	v := New(16000, 1)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := v.CalculateRMS(frameOf(tt.value, 64))
			if math.Abs(got-tt.want) > 1e-9 {
				t.Errorf("CalculateRMS = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCalculateRMSEmpty(t *testing.T) {
	v := New(16000, 1)
	if got := v.CalculateRMS(nil); got != 0 {
		t.Errorf("CalculateRMS(nil) = %v, want 0", got)
	}
}

func TestNewUnknownModeUsesFallback(t *testing.T) {
	v := New(16000, 99)
	if v.sensitivity != fallbackSensitivity {
		t.Errorf("sensitivity = %v, want fallback %v", v.sensitivity, fallbackSensitivity)
	}
}

func TestIsSpeechUsesInitialThreshold(t *testing.T) {
	v := New(16000, 1)

	if v.IsSpeech(frameOf(16384, 64)) != true {
		t.Error("loud frame should be detected as speech")
	}
	if v.IsSpeech(frameOf(0, 64)) != false {
		t.Error("silent frame should not be detected as speech")
	}
}

func TestCalibrateSetsThreshold(t *testing.T) {
	v := New(16000, 1)

	// 16-bit value 327 ≈ 0.00998 normalised RMS, comfortably below the loud test frame.
	const noise = int16(327)
	for i := 0; i < calibrationSampleThreshold; i++ {
		v.Calibrate(frameOf(noise, 64))
	}

	// With identical frames the standard deviation is zero, so the threshold
	// collapses to the mean noise level.
	wantThreshold := float64(noise) / 32768.0
	if math.Abs(v.Threshold()-wantThreshold) > 1e-6 {
		t.Errorf("Threshold = %v, want ~%v", v.Threshold(), wantThreshold)
	}
	if math.Abs(v.NoiseLevel()-wantThreshold) > 1e-6 {
		t.Errorf("NoiseLevel = %v, want ~%v", v.NoiseLevel(), wantThreshold)
	}

	if !v.IsSpeech(frameOf(16384, 64)) {
		t.Error("loud frame should be speech after calibration")
	}
	if v.IsSpeech(frameOf(noise, 64)) {
		t.Error("noise-level frame should not be speech after calibration")
	}
}

func TestUpdateThresholdTracksNoise(t *testing.T) {
	v := New(16000, 1)
	for i := 0; i < calibrationSampleThreshold; i++ {
		v.Calibrate(frameOf(327, 64))
	}

	before := v.NoiseLevel()
	// Feed louder (but still sub-speech) noise; the EMA should raise the floor.
	for i := 0; i < 50; i++ {
		v.UpdateThreshold(frameOf(500, 64))
	}
	if v.NoiseLevel() <= before {
		t.Errorf("NoiseLevel did not increase: before=%v after=%v", before, v.NoiseLevel())
	}
}

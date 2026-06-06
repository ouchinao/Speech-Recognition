// Package vad implements voice activity detection (VAD). It is a pure domain
// package: it depends only on the standard library and is free of any I/O or
// third-party framework, which keeps the core detection logic easy to test.
package vad

import (
	"encoding/binary"
	"math"
)

const (
	// calibrationSampleThreshold is the number of frames collected before the
	// initial noise threshold is derived during calibration.
	calibrationSampleThreshold = 100
	// initialThreshold is the RMS threshold used before calibration completes.
	initialThreshold = 0.02
	// emaAlpha is the smoothing factor of the exponential moving average used
	// to track ambient noise (10% new sample, 90% prior estimate).
	emaAlpha = 0.1
	// fallbackSensitivity is used when an unknown mode is requested.
	fallbackSensitivity = 2.5
)

// sensitivityByMode maps a VAD mode to the standard-deviation multiplier that
// derives the speech threshold. Larger values make detection stricter.
var sensitivityByMode = map[int]float64{
	0: 1.5, // lenient
	1: 2.0, // moderate
	2: 2.5, // stricter
	3: 3.5, // strictest
}

// AdaptiveVAD detects speech by comparing the short-term RMS energy of an audio
// frame against a threshold that adapts to the surrounding noise floor.
type AdaptiveVAD struct {
	sampleRate      int
	sensitivity     float64
	threshold       float64
	noiseLevel      float64
	noiseStdDev     float64
	calibrationData []float64
	alpha           float64
}

// New returns an AdaptiveVAD for the given sample rate and sensitivity mode.
// Unknown modes fall back to a moderate sensitivity.
func New(sampleRate, mode int) *AdaptiveVAD {
	sensitivity, ok := sensitivityByMode[mode]
	if !ok {
		sensitivity = fallbackSensitivity
	}

	return &AdaptiveVAD{
		sampleRate:      sampleRate,
		sensitivity:     sensitivity,
		threshold:       initialThreshold,
		calibrationData: make([]float64, 0, 1000),
		alpha:           emaAlpha,
	}
}

// Calibrate records the energy of an ambient-noise frame. Once enough samples
// have been collected it computes the noise floor and the speech threshold.
func (v *AdaptiveVAD) Calibrate(frame []byte) {
	rms := v.CalculateRMS(frame)
	v.calibrationData = append(v.calibrationData, rms)

	if len(v.calibrationData) >= calibrationSampleThreshold {
		mean, stdDev := stats(v.calibrationData)
		v.noiseLevel = mean
		v.noiseStdDev = stdDev
		v.threshold = mean + stdDev*v.sensitivity
	}
}

// CalculateRMS returns the root-mean-square amplitude of a little-endian 16-bit
// PCM frame, normalised to the range [0, 1].
func (v *AdaptiveVAD) CalculateRMS(frame []byte) float64 {
	samples := len(frame) / 2
	if samples == 0 {
		return 0
	}

	var sum float64
	for i := 0; i < samples; i++ {
		sample := int16(binary.LittleEndian.Uint16(frame[i*2:]))
		normalized := float64(sample) / 32768.0
		sum += normalized * normalized
	}

	return math.Sqrt(sum / float64(samples))
}

// IsSpeech reports whether the frame's energy exceeds the current threshold.
func (v *AdaptiveVAD) IsSpeech(frame []byte) bool {
	return v.CalculateRMS(frame) > v.threshold
}

// UpdateThreshold folds a silent frame into the noise estimate using an
// exponential moving average and recomputes the threshold. It should be called
// only for frames classified as non-speech.
func (v *AdaptiveVAD) UpdateThreshold(frame []byte) {
	rms := v.CalculateRMS(frame)
	v.noiseLevel = (1-v.alpha)*v.noiseLevel + v.alpha*rms
	v.threshold = v.noiseLevel + v.noiseStdDev*v.sensitivity
}

// Threshold returns the current speech-detection threshold.
func (v *AdaptiveVAD) Threshold() float64 {
	return v.threshold
}

// NoiseLevel returns the current estimate of the ambient noise floor.
func (v *AdaptiveVAD) NoiseLevel() float64 {
	return v.noiseLevel
}

// stats returns the mean and population standard deviation of data.
func stats(data []float64) (mean, stdDev float64) {
	if len(data) == 0 {
		return 0, 0
	}

	var sum float64
	for _, val := range data {
		sum += val
	}
	mean = sum / float64(len(data))

	var variance float64
	for _, val := range data {
		diff := val - mean
		variance += diff * diff
	}
	stdDev = math.Sqrt(variance / float64(len(data)))

	return mean, stdDev
}

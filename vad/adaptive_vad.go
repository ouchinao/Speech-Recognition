package vad

import (
	"math"
)

type AdaptiveVAD struct {
	sampleRate      int
	mode            int
	threshold       float64
	noiseLevel      float64
	noiseStdDev     float64
	calibrationData []float64
	alpha           float64 // exponential moving average coefficient
}

func NewAdaptiveVAD(sampleRate, mode int) *AdaptiveVAD {
	// sensitivity coefficient per mode: 0=lenient, 1=moderate, 2=stricter (default), 3=strictest
	sensitivity := map[int]float64{
		0: 1.5,
		1: 2.0,
		2: 2.5,
		3: 3.5,
	}

	coefficient := sensitivity[mode]
	if coefficient == 0 {
		coefficient = 2.5
	}

	return &AdaptiveVAD{
		sampleRate:      sampleRate,
		mode:            mode,
		threshold:       0.02,
		calibrationData: make([]float64, 0, 1000),
		alpha:           0.1, // weight 10% new sample, 90% prior estimate
	}
}

func (v *AdaptiveVAD) Calibrate(buffer []byte) {
	rms := v.CalculateRMS(buffer)
	v.calibrationData = append(v.calibrationData, rms)

	// once enough samples are collected, compute the threshold
	if len(v.calibrationData) >= 100 {
		mean, stdDev := v.calculateStats(v.calibrationData)
		v.noiseLevel = mean
		v.noiseStdDev = stdDev

		// threshold = mean + (stdDev * mode coefficient)
		sensitivity := map[int]float64{0: 1.5, 1: 2.0, 2: 2.5, 3: 3.5}
		v.threshold = mean + (stdDev * sensitivity[v.mode])
	}
}

func (v *AdaptiveVAD) CalculateRMS(buffer []byte) float64 {
	// 16-bit PCM -> float64
	var sum float64
	samples := len(buffer) / 2

	for i := 0; i < samples; i++ {
		sample := int16(buffer[i*2]) | (int16(buffer[i*2+1]) << 8)
		normalized := float64(sample) / 32768.0
		sum += normalized * normalized
	}

	return math.Sqrt(sum / float64(samples))
}

func (v *AdaptiveVAD) IsSpeech(buffer []byte) bool {
	rms := v.CalculateRMS(buffer)
	return rms > v.threshold
}

func (v *AdaptiveVAD) UpdateThreshold(buffer []byte) {
	rms := v.CalculateRMS(buffer)

	// update noise level via exponential moving average
	v.noiseLevel = (1-v.alpha)*v.noiseLevel + v.alpha*rms

	sensitivity := map[int]float64{0: 1.5, 1: 2.0, 2: 2.5, 3: 3.5}
	v.threshold = v.noiseLevel + (v.noiseStdDev * sensitivity[v.mode])
}

func (v *AdaptiveVAD) GetThreshold() float64 {
	return v.threshold
}

func (v *AdaptiveVAD) GetNoiseLevel() float64 {
	return v.noiseLevel
}

func (v *AdaptiveVAD) calculateStats(data []float64) (mean, stdDev float64) {
	sum := 0.0
	for _, val := range data {
		sum += val
	}
	mean = sum / float64(len(data))

	variance := 0.0
	for _, val := range data {
		diff := val - mean
		variance += diff * diff
	}
	stdDev = math.Sqrt(variance / float64(len(data)))

	return mean, stdDev
}

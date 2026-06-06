package recognition

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Frame markers used by the fakes to decide speech vs. silence.
var (
	speechFrame  = []byte{1}
	silenceFrame = []byte{0}
)

type fakeSource struct {
	frames [][]byte
	i      int
	cancel context.CancelFunc
}

func (f *fakeSource) Read() ([]byte, error) {
	if f.i >= len(f.frames) {
		if f.cancel != nil {
			f.cancel()
		}
		return nil, errors.New("source exhausted")
	}
	frame := f.frames[f.i]
	f.i++
	return frame, nil
}

type fakeDetector struct {
	calibrateCount int
	updateCount    int
}

func (d *fakeDetector) Calibrate(frame []byte)            { d.calibrateCount++ }
func (d *fakeDetector) IsSpeech(frame []byte) bool        { return len(frame) > 0 && frame[0] == 1 }
func (d *fakeDetector) CalculateRMS(frame []byte) float64 { return 0.5 }
func (d *fakeDetector) UpdateThreshold(frame []byte)      { d.updateCount++ }
func (d *fakeDetector) Threshold() float64                { return 0.1 }
func (d *fakeDetector) NoiseLevel() float64               { return 0.05 }

type fakeRecognizer struct {
	complete bool
	final    string
	partial  string
}

func (r *fakeRecognizer) AcceptWaveform(frame []byte) bool { return r.complete }
func (r *fakeRecognizer) Result() string                   { return r.final }
func (r *fakeRecognizer) PartialResult() string            { return r.partial }

type fakePrinter struct {
	finals   []string
	partials []string
	statuses int
}

func (p *fakePrinter) Final(text string)             { p.finals = append(p.finals, text) }
func (p *fakePrinter) Partial(text string)           { p.partials = append(p.partials, text) }
func (p *fakePrinter) Status(rms, threshold float64) { p.statuses++ }

func TestRunFinalResult(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	source := &fakeSource{frames: [][]byte{speechFrame, silenceFrame}, cancel: cancel}
	detector := &fakeDetector{}
	rec := &fakeRecognizer{complete: true, final: "hello"}
	printer := &fakePrinter{}

	err := NewService(source, detector, rec, printer).Run(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v, want context.Canceled", err)
	}

	if len(printer.finals) != 1 || printer.finals[0] != "hello" {
		t.Errorf("finals = %v, want [hello]", printer.finals)
	}
	if detector.updateCount != 1 {
		t.Errorf("updateCount = %d, want 1 (one silence frame)", detector.updateCount)
	}
	if printer.statuses != 1 {
		t.Errorf("statuses = %d, want 1", printer.statuses)
	}
}

func TestRunPartialResult(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	source := &fakeSource{frames: [][]byte{speechFrame}, cancel: cancel}
	rec := &fakeRecognizer{complete: false, partial: "he"}
	printer := &fakePrinter{}

	_ = NewService(source, &fakeDetector{}, rec, printer).Run(ctx)

	if len(printer.partials) != 1 || printer.partials[0] != "he" {
		t.Errorf("partials = %v, want [he]", printer.partials)
	}
	if len(printer.finals) != 0 {
		t.Errorf("finals = %v, want empty", printer.finals)
	}
}

func TestCalibrateStopsOnCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	detector := &fakeDetector{}
	source := &fakeSource{frames: [][]byte{silenceFrame, silenceFrame}}

	err := NewService(source, detector, &fakeRecognizer{}, &fakePrinter{}).Calibrate(ctx, time.Second)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Calibrate error = %v, want context.Canceled", err)
	}
	if detector.calibrateCount != 0 {
		t.Errorf("calibrateCount = %d, want 0 on cancelled context", detector.calibrateCount)
	}
}

func TestCalibrateSamplesNoise(t *testing.T) {
	source := &fakeSource{frames: make([][]byte, 100)}
	for i := range source.frames {
		source.frames[i] = silenceFrame
	}
	detector := &fakeDetector{}

	err := NewService(source, detector, &fakeRecognizer{}, &fakePrinter{}).Calibrate(context.Background(), 10*time.Millisecond)
	if err != nil {
		t.Fatalf("Calibrate error = %v", err)
	}
	if detector.calibrateCount == 0 {
		t.Error("calibrateCount = 0, want at least one sample")
	}
}

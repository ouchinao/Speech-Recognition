// Package recognizer provides an infrastructure adapter around the VOSK speech
// recognition engine. It hides the cgo-based VOSK API behind a small, testable
// surface and returns plain text rather than raw JSON.
//
// A Model is the expensive, read-only resource; load it once and create a
// lightweight Recognizer per audio stream from it.
package recognizer

import (
	"encoding/json"
	"fmt"

	vosk "github.com/alphacep/vosk-api/go"
)

// Model is a loaded VOSK acoustic model. It is safe to create many recognizers
// from a single model, so load it once and share it across streams.
type Model struct {
	model *vosk.VoskModel
}

// LoadModel loads the VOSK model at path. The returned Model must be released
// with Close once all recognizers created from it are done.
func LoadModel(path string) (*Model, error) {
	m, err := vosk.NewModel(path)
	if err != nil {
		return nil, fmt.Errorf("load model %q: %w", path, err)
	}
	return &Model{model: m}, nil
}

// NewRecognizer creates a recognizer for the given sample rate from the model.
// The returned Recognizer must be closed with Close.
func (m *Model) NewRecognizer(sampleRate int) (*Recognizer, error) {
	rec, err := vosk.NewRecognizer(m.model, float64(sampleRate))
	if err != nil {
		return nil, fmt.Errorf("create recognizer: %w", err)
	}
	rec.SetWords(1)
	return &Recognizer{recognizer: rec}, nil
}

// Close releases the model. Recognizers created from it must be closed first.
func (m *Model) Close() error {
	m.model.Free()
	return nil
}

// Recognizer wraps a VOSK recognizer and converts PCM audio to text.
type Recognizer struct {
	recognizer *vosk.VoskRecognizer
}

// AcceptWaveform feeds a PCM frame to the engine and reports whether the
// current utterance is complete (i.e. a final result is available).
func (r *Recognizer) AcceptWaveform(frame []byte) (complete bool) {
	return r.recognizer.AcceptWaveform(frame) == 1
}

// Result returns the final recognised text for the completed utterance.
func (r *Recognizer) Result() string {
	return extractText(r.recognizer.Result())
}

// PartialResult returns the in-progress recognised text for the current utterance.
func (r *Recognizer) PartialResult() string {
	return extractText(r.recognizer.PartialResult())
}

// FinalResult flushes the recognizer and returns the final text for any audio
// buffered since the last final result. Call it once when the audio ends.
func (r *Recognizer) FinalResult() string {
	return extractText(r.recognizer.FinalResult())
}

// Close releases the underlying VOSK recognizer (but not the model it came
// from). It always succeeds; the error return satisfies the io.Closer convention.
func (r *Recognizer) Close() error {
	r.recognizer.Free()
	return nil
}

// extractText pulls the human-readable text out of a VOSK JSON result, handling
// both final ("text") and partial ("partial") payloads.
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
	return result.Partial
}

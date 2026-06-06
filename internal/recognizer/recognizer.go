// Package recognizer provides an infrastructure adapter around the VOSK speech
// recognition engine. It hides the cgo-based VOSK API behind a small, testable
// surface and returns plain text rather than raw JSON.
package recognizer

import (
	"encoding/json"
	"fmt"

	vosk "github.com/alphacep/vosk-api/go"
)

// Recognizer wraps a VOSK model and recognizer and converts PCM audio to text.
type Recognizer struct {
	model      *vosk.VoskModel
	recognizer *vosk.VoskRecognizer
}

// New loads the VOSK model at modelPath and creates a recognizer configured for
// the given sample rate. The returned Recognizer must be closed with Close.
func New(modelPath string, sampleRate int) (*Recognizer, error) {
	model, err := vosk.NewModel(modelPath)
	if err != nil {
		return nil, fmt.Errorf("load model %q: %w", modelPath, err)
	}

	rec, err := vosk.NewRecognizer(model, float64(sampleRate))
	if err != nil {
		model.Free()
		return nil, fmt.Errorf("create recognizer: %w", err)
	}
	rec.SetWords(1)

	return &Recognizer{model: model, recognizer: rec}, nil
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

// Close releases the underlying VOSK resources. It always succeeds and returns
// nil; the error return exists to satisfy the io.Closer convention.
func (r *Recognizer) Close() error {
	r.recognizer.Free()
	r.model.Free()
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

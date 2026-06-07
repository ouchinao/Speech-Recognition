package recognition

import (
	"context"
	"errors"
	"fmt"
	"io"
)

// Streamer runs request-scoped streaming recognition: it feeds every audio
// frame from the source to the recognizer and emits partial and final
// transcripts until the source is exhausted.
//
// Unlike Service it performs no voice-activity gating — it relies on the
// recognizer's own endpointing — and it terminates gracefully when the source
// returns io.EOF (the client finished sending), flushing the final result.
// This makes it the natural use case behind a streaming network API.
type Streamer struct {
	source     AudioSource
	recognizer Recognizer
	printer    Printer
}

// NewStreamer wires the streaming use case with its collaborators.
func NewStreamer(source AudioSource, recognizer Recognizer, printer Printer) *Streamer {
	return &Streamer{source: source, recognizer: recognizer, printer: printer}
}

// Run consumes audio until the source returns io.EOF (a graceful end, after
// which the final result is flushed) or ctx is cancelled.
func (s *Streamer) Run(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		frame, err := s.source.Read()
		if errors.Is(err, io.EOF) {
			if text := s.recognizer.FinalResult(); text != "" {
				s.printer.Final(text)
			}
			return nil
		}
		if err != nil {
			return fmt.Errorf("read audio: %w", err)
		}

		if s.recognizer.AcceptWaveform(frame) {
			if text := s.recognizer.Result(); text != "" {
				s.printer.Final(text)
			}
		} else if text := s.recognizer.PartialResult(); text != "" {
			s.printer.Partial(text)
		}
	}
}

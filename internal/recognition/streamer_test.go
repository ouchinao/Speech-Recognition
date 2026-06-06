package recognition

import (
	"context"
	"io"
	"testing"
)

func TestStreamerPartialsThenFinalFlush(t *testing.T) {
	source := &fakeSource{
		frames: [][]byte{{1}, {1}},
		endErr: io.EOF,
	}
	rec := &fakeRecognizer{complete: false, partial: "he", flush: "hello"}
	printer := &fakePrinter{}

	if err := NewStreamer(source, rec, printer).Run(context.Background()); err != nil {
		t.Fatalf("Run error = %v", err)
	}

	if got := printer.partials; len(got) != 2 || got[0] != "he" {
		t.Errorf("partials = %v, want two %q", got, "he")
	}
	if got := printer.finals; len(got) != 1 || got[0] != "hello" {
		t.Errorf("finals = %v, want [hello] flushed on EOF", got)
	}
}

func TestStreamerFinalMidStream(t *testing.T) {
	source := &fakeSource{frames: [][]byte{{1}}, endErr: io.EOF}
	rec := &fakeRecognizer{complete: true, final: "done"} // flush is empty
	printer := &fakePrinter{}

	if err := NewStreamer(source, rec, printer).Run(context.Background()); err != nil {
		t.Fatalf("Run error = %v", err)
	}

	if got := printer.finals; len(got) != 1 || got[0] != "done" {
		t.Errorf("finals = %v, want [done]", got)
	}
	if len(printer.partials) != 0 {
		t.Errorf("partials = %v, want none", printer.partials)
	}
}

func TestStreamerStopsOnCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	source := &fakeSource{frames: [][]byte{{1}}, endErr: io.EOF}
	err := NewStreamer(source, &fakeRecognizer{}, &fakePrinter{}).Run(ctx)
	if err != context.Canceled {
		t.Fatalf("Run error = %v, want context.Canceled", err)
	}
}

func TestStreamerPropagatesReadError(t *testing.T) {
	source := &fakeSource{frames: nil} // exhausted immediately, generic (non-EOF) error
	err := NewStreamer(source, &fakeRecognizer{}, &fakePrinter{}).Run(context.Background())
	if err == nil {
		t.Fatal("Run error = nil, want a wrapped read error")
	}
}

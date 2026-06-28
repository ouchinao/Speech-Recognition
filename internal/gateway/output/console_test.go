package output

import (
	"bytes"
	"strings"
	"testing"
)

func TestConsoleFinal(t *testing.T) {
	var buf bytes.Buffer
	NewConsole(&buf).Final("こんにちは")

	got := buf.String()
	if !strings.Contains(got, "[final] こんにちは") {
		t.Errorf("Final output = %q, want it to contain %q", got, "[final] こんにちは")
	}
	if !strings.HasSuffix(got, "\n") {
		t.Errorf("Final output = %q, want trailing newline", got)
	}
}

func TestConsolePartial(t *testing.T) {
	var buf bytes.Buffer
	NewConsole(&buf).Partial("こん")

	if got := buf.String(); !strings.Contains(got, "[partial] こん") {
		t.Errorf("Partial output = %q, want it to contain %q", got, "[partial] こん")
	}
}

func TestConsoleStatus(t *testing.T) {
	var buf bytes.Buffer
	NewConsole(&buf).Status(0.123, 0.456)

	got := buf.String()
	for _, want := range []string{"0.123", "0.456", "speech detected"} {
		if !strings.Contains(got, want) {
			t.Errorf("Status output = %q, want it to contain %q", got, want)
		}
	}
}

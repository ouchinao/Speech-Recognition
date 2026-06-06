// Package output renders recognition results to a terminal. It isolates all
// presentation concerns (formatting, ANSI escape codes) from the use case.
package output

import (
	"fmt"
	"io"
)

// Console writes recognition output to an io.Writer, using ANSI escape codes to
// update the current terminal line in place for partial results and status.
type Console struct {
	w io.Writer
}

// NewConsole returns a Console that writes to w.
func NewConsole(w io.Writer) *Console {
	return &Console{w: w}
}

// Final prints a finalised recognition result on its own line.
func (c *Console) Final(text string) {
	fmt.Fprintf(c.w, "\r\033[K[final] %s\n", text)
}

// Partial overwrites the current line with an in-progress recognition result.
func (c *Console) Partial(text string) {
	fmt.Fprintf(c.w, "\r[partial] %s", text)
}

// Status overwrites the current line with live VAD telemetry.
func (c *Console) Status(rms, threshold float64) {
	fmt.Fprintf(c.w, "\rRMS: %.3f | threshold: %.3f | speech detected", rms, threshold)
}

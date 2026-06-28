// Package audio provides an infrastructure adapter that captures microphone
// input through PortAudio and exposes it as little-endian 16-bit PCM frames.
package audio

import (
	"encoding/binary"
	"fmt"

	"github.com/gordonklaus/portaudio"
)

// Capture reads PCM audio from the default input device. It implements the
// audio source consumed by the recognition use case.
type Capture struct {
	stream          *portaudio.Stream
	buffer          []int16
	framesPerBuffer int
}

// New initialises PortAudio, opens the default input stream and starts it.
// The returned Capture must be closed with Close to release device resources.
func New(sampleRate, framesPerBuffer int) (*Capture, error) {
	if err := portaudio.Initialize(); err != nil {
		return nil, fmt.Errorf("initialize portaudio: %w", err)
	}

	buffer := make([]int16, framesPerBuffer)
	stream, err := portaudio.OpenDefaultStream(1, 0, float64(sampleRate), framesPerBuffer, buffer)
	if err != nil {
		_ = portaudio.Terminate()
		return nil, fmt.Errorf("open default stream: %w", err)
	}

	if err := stream.Start(); err != nil {
		_ = stream.Close()
		_ = portaudio.Terminate()
		return nil, fmt.Errorf("start stream: %w", err)
	}

	return &Capture{
		stream:          stream,
		buffer:          buffer,
		framesPerBuffer: framesPerBuffer,
	}, nil
}

// Read blocks until a frame is available and returns it as little-endian
// 16-bit PCM bytes.
func (c *Capture) Read() ([]byte, error) {
	if err := c.stream.Read(); err != nil {
		return nil, fmt.Errorf("read stream: %w", err)
	}

	out := make([]byte, len(c.buffer)*2)
	for i, sample := range c.buffer {
		binary.LittleEndian.PutUint16(out[i*2:], uint16(sample))
	}
	return out, nil
}

// Close stops and closes the stream and terminates PortAudio. It returns the
// first error encountered while still attempting every teardown step.
func (c *Capture) Close() error {
	var firstErr error
	if err := c.stream.Stop(); err != nil && firstErr == nil {
		firstErr = fmt.Errorf("stop stream: %w", err)
	}
	if err := c.stream.Close(); err != nil && firstErr == nil {
		firstErr = fmt.Errorf("close stream: %w", err)
	}
	if err := portaudio.Terminate(); err != nil && firstErr == nil {
		firstErr = fmt.Errorf("terminate portaudio: %w", err)
	}
	return firstErr
}

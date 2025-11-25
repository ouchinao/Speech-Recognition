package audio

import (
	"github.com/gordonklaus/portaudio"
)

type Capture struct {
	stream          *portaudio.Stream
	buffer          []int16
	framesPerBuffer int
}

func NewCapture(sampleRate, framesPerBuffer int) (*Capture, error) {
	if err := portaudio.Initialize(); err != nil {
		return nil, err
	}

	buffer := make([]int16, framesPerBuffer)
	stream, err := portaudio.OpenDefaultStream(1, 0, float64(sampleRate), framesPerBuffer, buffer)
	if err != nil {
		return nil, err
	}

	if err := stream.Start(); err != nil {
		return nil, err
	}

	return &Capture{
		stream:          stream,
		buffer:          buffer,
		framesPerBuffer: framesPerBuffer,
	}, nil
}

func (c *Capture) Read() ([]byte, error) {
	if err := c.stream.Read(); err != nil {
		return nil, err
	}

	// int16 → []byte変換
	byteBuffer := make([]byte, len(c.buffer)*2)
	for i, sample := range c.buffer {
		byteBuffer[i*2] = byte(sample)
		byteBuffer[i*2+1] = byte(sample >> 8)
	}

	return byteBuffer, nil
}

func (c *Capture) Close() error {
	if err := c.stream.Stop(); err != nil {
		return err
	}
	if err := c.stream.Close(); err != nil {
		return err
	}
	return portaudio.Terminate()
}

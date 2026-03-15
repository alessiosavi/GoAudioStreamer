package audio

import (
	"fmt"
	"log/slog"

	"github.com/gordonklaus/portaudio"

	"voxlink/internal/codec"
)

// Capture reads audio from the microphone via PortAudio into a ring buffer.
type Capture struct {
	stream *portaudio.Stream
	buf    *RingBuf
	logger *slog.Logger
}

func NewCapture(buf *RingBuf, logger *slog.Logger) (*Capture, error) {
	return &Capture{buf: buf, logger: logger}, nil
}

func (c *Capture) Start() error {
	stream, err := portaudio.OpenDefaultStream(
		1, 0, float64(codec.SampleRate), codec.FrameSize,
		func(in []int16) {
			var frame [codec.FrameSize]int16
			copy(frame[:], in)
			c.buf.Write(frame)
		},
	)
	if err != nil {
		return fmt.Errorf("open audio stream: %w", err)
	}
	c.stream = stream
	return stream.Start()
}

func (c *Capture) Stop() error {
	if c.stream != nil {
		c.stream.Stop()
		return c.stream.Close()
	}
	return nil
}

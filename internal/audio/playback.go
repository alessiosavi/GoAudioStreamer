package audio

import (
	"fmt"
	"log/slog"

	"github.com/gordonklaus/portaudio"

	"voxlink/internal/codec"
)

// Playback writes mixed audio to the speaker via PortAudio.
type Playback struct {
	stream *portaudio.Stream
	mixer  *Mixer
	logger *slog.Logger
}

func NewPlayback(mixer *Mixer, logger *slog.Logger) (*Playback, error) {
	return &Playback{mixer: mixer, logger: logger}, nil
}

func (p *Playback) Start() error {
	stream, err := portaudio.OpenDefaultStream(
		0, 1, float64(codec.SampleRate), codec.FrameSize,
		func(out []int16) {
			frame := p.mixer.Mix()
			copy(out, frame[:])
		},
	)
	if err != nil {
		return fmt.Errorf("open audio stream: %w", err)
	}
	p.stream = stream
	return stream.Start()
}

func (p *Playback) Stop() error {
	if p.stream != nil {
		p.stream.Stop()
		return p.stream.Close()
	}
	return nil
}

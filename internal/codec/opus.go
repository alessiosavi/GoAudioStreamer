package codec

import (
	"fmt"

	"gopkg.in/hraban/opus.v2"
)

const (
	SampleRate    = 48000
	Channels      = 1
	FrameSize     = 960   // 20ms at 48kHz
	Bitrate       = 24000 // 24 kbps
	MaxPacketSize = 4000
)

// Encoder wraps an Opus encoder with VoxLink parameters.
type Encoder struct {
	enc *opus.Encoder
}

// NewEncoder creates an Opus encoder configured for VoIP with DTX.
func NewEncoder() (*Encoder, error) {
	enc, err := opus.NewEncoder(SampleRate, Channels, opus.AppVoIP)
	if err != nil {
		return nil, fmt.Errorf("opus encoder: %w", err)
	}
	if err := enc.SetBitrate(Bitrate); err != nil {
		return nil, fmt.Errorf("set bitrate: %w", err)
	}
	if err := enc.SetDTX(true); err != nil {
		return nil, fmt.Errorf("set DTX: %w", err)
	}
	return &Encoder{enc: enc}, nil
}

// Encode encodes a FrameSize slice of int16 PCM samples into Opus.
func (e *Encoder) Encode(pcm []int16) ([]byte, error) {
	if len(pcm) != FrameSize {
		return nil, fmt.Errorf("expected %d samples, got %d", FrameSize, len(pcm))
	}
	buf := make([]byte, MaxPacketSize)
	n, err := e.enc.Encode(pcm, buf)
	if err != nil {
		return nil, fmt.Errorf("opus encode: %w", err)
	}
	return buf[:n], nil
}

// Close is a no-op (Opus encoder has no resources to free) but exists for symmetry.
func (e *Encoder) Close() {}

// Decoder wraps an Opus decoder with VoxLink parameters.
type Decoder struct {
	dec *opus.Decoder
}

// NewDecoder creates an Opus decoder.
func NewDecoder() (*Decoder, error) {
	dec, err := opus.NewDecoder(SampleRate, Channels)
	if err != nil {
		return nil, fmt.Errorf("opus decoder: %w", err)
	}
	return &Decoder{dec: dec}, nil
}

// Decode decodes an Opus packet into FrameSize int16 PCM samples.
func (d *Decoder) Decode(data []byte) ([]int16, error) {
	pcm := make([]int16, FrameSize)
	n, err := d.dec.Decode(data, pcm)
	if err != nil {
		return nil, fmt.Errorf("opus decode: %w", err)
	}
	return pcm[:n], nil
}

// Close is a no-op but exists for symmetry.
func (d *Decoder) Close() {}

package audio

import (
	"context"
	"log/slog"

	"voxlink/internal/audio/rnnoise"
	"voxlink/internal/codec"
)

// Pipeline orchestrates: RingBuf -> RNNoise (2x480) -> Opus Encode -> callback.
type Pipeline struct {
	ringBuf  *RingBuf
	denoiser *rnnoise.Denoiser
	encoder  *codec.Encoder
	logger   *slog.Logger
	denoise  bool
}

func NewPipeline(ringBuf *RingBuf, logger *slog.Logger) *Pipeline {
	p := &Pipeline{ringBuf: ringBuf, logger: logger, denoise: true}

	enc, err := codec.NewEncoder()
	if err != nil {
		logger.Error("opus encoder init failed", "err", err)
		return p
	}
	p.encoder = enc

	d, err := rnnoise.New()
	if err != nil {
		logger.Warn("RNNoise init failed, denoising disabled", "err", err)
		p.denoise = false
	} else {
		p.denoiser = d
	}

	return p
}

func (p *Pipeline) SetDenoise(enabled bool) {
	p.denoise = enabled && p.denoiser != nil
}

// Run reads frames from the ring buffer, denoises, encodes, and calls onPacket.
// Blocks until ctx is cancelled.
func (p *Pipeline) Run(ctx context.Context, onPacket func([]byte), onVAD func(float32)) {
	var (
		floatIn  [rnnoise.FrameSize]float32
		floatOut [rnnoise.FrameSize]float32
	)

	for {
		// Wait for a frame or context cancellation — no busy loop.
		select {
		case <-ctx.Done():
			return
		case <-p.ringBuf.Notify():
		}

		frame, ok := p.ringBuf.Read()
		if !ok {
			continue
		}

		var pcm [codec.FrameSize]int16

		if p.denoise && p.denoiser != nil {
			rnnoise.Int16ToFloat32(frame[:480], floatIn[:])
			vad1, _ := p.denoiser.ProcessFrame(floatOut[:], floatIn[:])
			rnnoise.Float32ToInt16(floatOut[:], pcm[:480])

			rnnoise.Int16ToFloat32(frame[480:], floatIn[:])
			vad2, _ := p.denoiser.ProcessFrame(floatOut[:], floatIn[:])
			rnnoise.Float32ToInt16(floatOut[:], pcm[480:])

			if onVAD != nil {
				onVAD((vad1 + vad2) / 2.0)
			}
		} else {
			pcm = frame
			if onVAD != nil {
				onVAD(1.0)
			}
		}

		if p.encoder == nil {
			continue
		}
		encoded, err := p.encoder.Encode(pcm[:])
		if err != nil {
			p.logger.Error("opus encode failed", "err", err)
			continue
		}
		onPacket(encoded)
	}
}

func (p *Pipeline) Close() {
	if p.denoiser != nil {
		p.denoiser.Close()
	}
}

package rnnoise

/*
#cgo CFLAGS: -I${SRCDIR}/rnnoise_src/include -I${SRCDIR}/rnnoise_src/src -DCOMPILE_OPUS
#include "rnnoise_src/include/rnnoise.h"
*/
import "C"
import (
	"fmt"
	"math"
	"unsafe"
)

// FrameSize is the number of samples per frame expected by RNNoise (10ms at 48kHz).
const FrameSize = 480

// Denoiser wraps an RNNoise DenoiseState for single-channel noise suppression.
type Denoiser struct {
	state *C.DenoiseState
}

// New allocates and returns a new Denoiser. Call Close when done.
func New() (*Denoiser, error) {
	state := C.rnnoise_create(nil)
	if state == nil {
		return nil, fmt.Errorf("rnnoise_create failed")
	}
	return &Denoiser{state: state}, nil
}

// Close frees the underlying C state.
func (d *Denoiser) Close() {
	if d.state != nil {
		C.rnnoise_destroy(d.state)
		d.state = nil
	}
}

// ProcessFrame denoises a single frame of FrameSize float32 samples.
// Values should be in int16 range (-32768..32767), NOT normalized [-1,1].
// Returns the VAD (voice activity detection) probability in [0.0, 1.0].
func (d *Denoiser) ProcessFrame(out, in []float32) (float32, error) {
	if len(in) != FrameSize {
		return 0, fmt.Errorf("input must be %d samples, got %d", FrameSize, len(in))
	}
	if len(out) < FrameSize {
		return 0, fmt.Errorf("output must be at least %d samples, got %d", FrameSize, len(out))
	}

	vad := C.rnnoise_process_frame(
		d.state,
		(*C.float)(unsafe.Pointer(&out[0])),
		(*C.float)(unsafe.Pointer(&in[0])),
	)
	return float32(vad), nil
}

// Int16ToFloat32 converts int16 PCM samples to float32 values (no normalization).
func Int16ToFloat32(in []int16, out []float32) {
	for i, s := range in {
		out[i] = float32(s)
	}
}

// Float32ToInt16 converts float32 values back to int16 with clamping.
func Float32ToInt16(in []float32, out []int16) {
	for i, s := range in {
		v := math.Round(float64(s))
		if v > 32767 {
			v = 32767
		} else if v < -32768 {
			v = -32768
		}
		out[i] = int16(v)
	}
}

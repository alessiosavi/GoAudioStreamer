package codec

import (
	"testing"
)

func TestOpusRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping CGo test in short mode")
	}

	enc, err := NewEncoder()
	if err != nil {
		t.Fatal(err)
	}
	defer enc.Close()

	dec, err := NewDecoder()
	if err != nil {
		t.Fatal(err)
	}
	defer dec.Close()

	// Generate a 960-sample sine wave frame (20ms at 48kHz).
	var input [FrameSize]int16
	for i := range input {
		// 440Hz sine wave, amplitude 8000
		input[i] = int16(8000.0 * sinApprox(float64(i)*440.0*2.0*3.14159/float64(SampleRate)))
	}

	// Encode
	encoded, err := enc.Encode(input[:])
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if len(encoded) == 0 {
		t.Fatal("encoded data is empty")
	}
	if len(encoded) > MaxPacketSize {
		t.Fatalf("encoded size %d exceeds MaxPacketSize %d", len(encoded), MaxPacketSize)
	}

	// Decode
	decoded, err := dec.Decode(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(decoded) != FrameSize {
		t.Fatalf("decoded length: got %d, want %d", len(decoded), FrameSize)
	}

	// Verify the decoded signal is not silence (Opus is lossy, so no exact match).
	maxAbs := int16(0)
	for _, s := range decoded {
		if s > maxAbs {
			maxAbs = s
		} else if -s > maxAbs {
			maxAbs = -s
		}
	}
	if maxAbs < 1000 {
		t.Fatalf("decoded signal is too quiet (maxAbs=%d), expected audible signal", maxAbs)
	}
}

func TestOpusEncodeSilence(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping CGo test in short mode")
	}

	enc, err := NewEncoder()
	if err != nil {
		t.Fatal(err)
	}
	defer enc.Close()

	// All zeros = silence
	silence := make([]int16, FrameSize)
	encoded, err := enc.Encode(silence)
	if err != nil {
		t.Fatalf("encode silence: %v", err)
	}
	// Opus with DTX should produce a very small packet for silence.
	if len(encoded) > 100 {
		t.Logf("warning: silence encoded to %d bytes (DTX may not have kicked in yet)", len(encoded))
	}
}

// sinApprox is a rough sine approximation for test signal generation.
func sinApprox(x float64) float64 {
	// Normalize to [0, 2*pi]
	for x > 6.283185 {
		x -= 6.283185
	}
	for x < 0 {
		x += 6.283185
	}
	// Bhaskara I approximation
	if x > 3.141593 {
		x -= 3.141593
		return -(16*x*(3.141593-x)) / (49.348 - 4*x*(3.141593-x))
	}
	return (16 * x * (3.141593 - x)) / (49.348 - 4*x*(3.141593-x))
}

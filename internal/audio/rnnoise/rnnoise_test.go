package rnnoise

import (
	"math"
	"testing"
)

func TestDenoiser_ProcessFrame(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping CGo test in short mode")
	}

	d, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	// RNNoise needs several frames to warm up before producing non-zero output.
	// Feed multiple frames of a tone + noise signal.
	const numFrames = 10
	var out [FrameSize]float32
	var vad float32
	for f := 0; f < numFrames; f++ {
		var in [FrameSize]float32
		for i := range in {
			sample := f*FrameSize + i
			tone := float32(10000.0 * math.Sin(float64(sample)*300.0*2.0*math.Pi/48000.0))
			noise := float32((sample*7 + 13) % 2000 - 1000)
			in[i] = tone + noise
		}
		vad, err = d.ProcessFrame(out[:], in[:])
		if err != nil {
			t.Fatal(err)
		}
	}

	if vad < 0.0 || vad > 1.0 {
		t.Fatalf("VAD probability out of range: %f", vad)
	}

	allZero := true
	for _, s := range out {
		if s != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		t.Fatal("output is all zeros — denoiser produced no output after warmup")
	}
}

func TestInt16ToFloat32_AndBack(t *testing.T) {
	in := []int16{0, 1, -1, 32767, -32768, 100, -100}
	floats := make([]float32, len(in))
	Int16ToFloat32(in, floats)

	for i, v := range floats {
		if v != float32(in[i]) {
			t.Errorf("index %d: got %f, want %f", i, v, float32(in[i]))
		}
	}

	out := make([]int16, len(floats))
	Float32ToInt16(floats, out)
	for i, v := range out {
		if v != in[i] {
			t.Errorf("round-trip index %d: got %d, want %d", i, v, in[i])
		}
	}
}

func TestFloat32ToInt16_Clamping(t *testing.T) {
	in := []float32{40000.0, -40000.0, 32767.5, -32768.5}
	out := make([]int16, len(in))
	Float32ToInt16(in, out)

	expected := []int16{32767, -32768, 32767, -32768}
	for i, v := range out {
		if v != expected[i] {
			t.Errorf("index %d: got %d, want %d", i, v, expected[i])
		}
	}
}

func TestDenoiser_FrameSizeValidation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping CGo test in short mode")
	}

	d, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	in := make([]float32, 100)
	out := make([]float32, FrameSize)
	_, err = d.ProcessFrame(out, in)
	if err == nil {
		t.Fatal("expected error for wrong input size")
	}
}

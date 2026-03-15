package audio

import (
	"testing"

	"voxlink/internal/codec"
)

func TestMixer_SingleStream(t *testing.T) {
	m := NewMixer()
	m.AddStream("peer-1")

	var frame [codec.FrameSize]int16
	for i := range frame {
		frame[i] = 1000
	}
	m.PushFrame("peer-1", frame)

	out := m.Mix()
	if out[0] != 1000 {
		t.Fatalf("got %d, want 1000", out[0])
	}
}

func TestMixer_TwoStreams(t *testing.T) {
	m := NewMixer()
	m.AddStream("peer-1")
	m.AddStream("peer-2")

	var f1, f2 [codec.FrameSize]int16
	for i := range f1 {
		f1[i] = 5000
		f2[i] = 3000
	}
	m.PushFrame("peer-1", f1)
	m.PushFrame("peer-2", f2)

	out := m.Mix()
	if out[0] != 8000 {
		t.Fatalf("got %d, want 8000", out[0])
	}
}

func TestMixer_Clipping(t *testing.T) {
	m := NewMixer()
	m.AddStream("peer-1")
	m.AddStream("peer-2")

	var f1, f2 [codec.FrameSize]int16
	for i := range f1 {
		f1[i] = 30000
		f2[i] = 20000
	}
	m.PushFrame("peer-1", f1)
	m.PushFrame("peer-2", f2)

	out := m.Mix()
	// 30000 + 20000 = 50000 > 32767, should clip.
	if out[0] != 32767 {
		t.Fatalf("got %d, want 32767 (clipped)", out[0])
	}
}

func TestMixer_NegativeClipping(t *testing.T) {
	m := NewMixer()
	m.AddStream("peer-1")
	m.AddStream("peer-2")

	var f1, f2 [codec.FrameSize]int16
	for i := range f1 {
		f1[i] = -30000
		f2[i] = -20000
	}
	m.PushFrame("peer-1", f1)
	m.PushFrame("peer-2", f2)

	out := m.Mix()
	if out[0] != -32768 {
		t.Fatalf("got %d, want -32768 (clipped)", out[0])
	}
}

func TestMixer_VolumeControl(t *testing.T) {
	m := NewMixer()
	m.AddStream("peer-1")
	m.SetVolume("peer-1", 0.5)

	var frame [codec.FrameSize]int16
	for i := range frame {
		frame[i] = 10000
	}
	m.PushFrame("peer-1", frame)

	out := m.Mix()
	// 10000 * 0.5 = 5000
	if out[0] != 5000 {
		t.Fatalf("got %d, want 5000", out[0])
	}
}

func TestMixer_RemoveStream(t *testing.T) {
	m := NewMixer()
	m.AddStream("peer-1")
	m.RemoveStream("peer-1")

	var frame [codec.FrameSize]int16
	frame[0] = 1000
	m.PushFrame("peer-1", frame) // should be silently ignored

	out := m.Mix()
	if out[0] != 0 {
		t.Fatalf("got %d, want 0 (stream removed)", out[0])
	}
}

func TestMixer_MissingFrame_OutputsSilence(t *testing.T) {
	m := NewMixer()
	m.AddStream("peer-1")
	// Don't push any frame.

	out := m.Mix()
	if out[0] != 0 {
		t.Fatalf("got %d, want 0 (no frame pushed)", out[0])
	}
}

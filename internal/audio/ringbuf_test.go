package audio

import (
	"sync"
	"testing"

	"voxlink/internal/codec"
)

func TestRingBuf_WriteRead(t *testing.T) {
	rb := NewRingBuf(4)

	var frame [codec.FrameSize]int16
	for i := range frame {
		frame[i] = int16(i % 100)
	}

	if !rb.Write(frame) {
		t.Fatal("write to empty buffer should succeed")
	}

	got, ok := rb.Read()
	if !ok {
		t.Fatal("read from non-empty buffer should succeed")
	}
	if got[0] != 0 || got[99] != 99 {
		t.Fatalf("data mismatch: got[0]=%d, got[99]=%d", got[0], got[99])
	}
}

func TestRingBuf_ReadEmpty(t *testing.T) {
	rb := NewRingBuf(4)
	_, ok := rb.Read()
	if ok {
		t.Fatal("read from empty buffer should return false")
	}
}

func TestRingBuf_Overflow_DropsOldest(t *testing.T) {
	rb := NewRingBuf(4)

	// Write 5 frames (capacity is 4, so frame 0 should be dropped).
	for i := 0; i < 5; i++ {
		var frame [codec.FrameSize]int16
		frame[0] = int16(i)
		rb.Write(frame)
	}

	// Should read frames 1, 2, 3, 4 (frame 0 was dropped).
	for expected := int16(1); expected <= 4; expected++ {
		got, ok := rb.Read()
		if !ok {
			t.Fatalf("expected frame %d, got empty", expected)
		}
		if got[0] != expected {
			t.Fatalf("got frame[0]=%d, want %d", got[0], expected)
		}
	}

	// Buffer should now be empty.
	_, ok := rb.Read()
	if ok {
		t.Fatal("buffer should be empty after reading all frames")
	}
}

func TestRingBuf_ConcurrentWriteRead(t *testing.T) {
	rb := NewRingBuf(4)
	const numFrames = 10000

	var wg sync.WaitGroup
	wg.Add(2)

	// Producer
	go func() {
		defer wg.Done()
		for i := 0; i < numFrames; i++ {
			var frame [codec.FrameSize]int16
			frame[0] = int16(i % 32000)
			rb.Write(frame)
		}
	}()

	// Consumer
	readCount := 0
	go func() {
		defer wg.Done()
		for i := 0; i < numFrames*2; i++ { // try more reads than writes
			if _, ok := rb.Read(); ok {
				readCount++
			}
		}
	}()

	wg.Wait()
	// We should have read at least some frames (not all — some may be dropped).
	if readCount == 0 {
		t.Fatal("consumer read 0 frames")
	}
	t.Logf("produced %d, consumed %d (dropped %d)", numFrames, readCount, numFrames-readCount)
}

package audio

import (
	"sync"

	"voxlink/internal/codec"
)

// RingBuf is a mutex-protected SPSC ring buffer for audio frames.
// One goroutine writes (PortAudio callback), one goroutine reads (processing pipeline).
// On overflow, the oldest frame is dropped. Uses a notify channel so the
// reader can block instead of busy-looping.
type RingBuf struct {
	mu     sync.Mutex
	frames [][codec.FrameSize]int16
	cap    int
	wIdx   int
	rIdx   int
	count  int
	notify chan struct{} // signaled on each write; capacity 1 to avoid blocking writer
}

// NewRingBuf creates a ring buffer with the given capacity (number of frames).
func NewRingBuf(capacity int) *RingBuf {
	return &RingBuf{
		frames: make([][codec.FrameSize]int16, capacity),
		cap:    capacity,
		notify: make(chan struct{}, 1),
	}
}

// Write adds a frame to the buffer. If the buffer is full, the oldest frame is dropped.
// Returns true if the write succeeded without dropping, false if a frame was dropped.
func (rb *RingBuf) Write(data [codec.FrameSize]int16) bool {
	rb.mu.Lock()
	dropped := false
	if rb.count >= rb.cap {
		// Drop oldest: advance read index.
		rb.rIdx = (rb.rIdx + 1) % rb.cap
		rb.count--
		dropped = true
	}
	rb.frames[rb.wIdx] = data
	rb.wIdx = (rb.wIdx + 1) % rb.cap
	rb.count++
	rb.mu.Unlock()

	// Non-blocking signal to reader.
	select {
	case rb.notify <- struct{}{}:
	default:
	}
	return !dropped
}

// Read retrieves the oldest frame from the buffer.
// Returns the frame and true, or a zero frame and false if empty.
func (rb *RingBuf) Read() ([codec.FrameSize]int16, bool) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if rb.count == 0 {
		return [codec.FrameSize]int16{}, false
	}

	data := rb.frames[rb.rIdx]
	rb.rIdx = (rb.rIdx + 1) % rb.cap
	rb.count--
	return data, true
}

// Notify returns a channel that receives a signal when a frame is written.
// Use this to avoid busy-looping in the consumer.
func (rb *RingBuf) Notify() <-chan struct{} {
	return rb.notify
}

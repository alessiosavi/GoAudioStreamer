package audio

import (
	"sync"

	"voxlink/internal/codec"
)

// Mixer combines N audio streams into one output frame with per-user volume.
type Mixer struct {
	mu      sync.Mutex
	streams map[string]*mixerStream
}

type mixerStream struct {
	volume float64
	frame  [codec.FrameSize]int16
	hasNew bool
}

// NewMixer creates an empty mixer.
func NewMixer() *Mixer {
	return &Mixer{
		streams: make(map[string]*mixerStream),
	}
}

// AddStream registers a new audio stream for a peer.
func (m *Mixer) AddStream(peerID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.streams[peerID] = &mixerStream{volume: 1.0}
}

// RemoveStream unregisters a peer's audio stream.
func (m *Mixer) RemoveStream(peerID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.streams, peerID)
}

// SetVolume sets the volume for a peer (0.0 = mute, 1.0 = full).
func (m *Mixer) SetVolume(peerID string, volume float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.streams[peerID]; ok {
		s.volume = volume
	}
}

// PushFrame pushes a decoded PCM frame for a peer. Replaces any unpicked frame.
func (m *Mixer) PushFrame(peerID string, frame [codec.FrameSize]int16) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.streams[peerID]; ok {
		s.frame = frame
		s.hasNew = true
	}
}

// Mix combines all pending frames into one output frame, applying volume and clipping.
// Frames that were not pushed since the last Mix call contribute silence.
func (m *Mixer) Mix() [codec.FrameSize]int16 {
	m.mu.Lock()
	defer m.mu.Unlock()

	var out [codec.FrameSize]int16
	for i := 0; i < codec.FrameSize; i++ {
		var sum int32
		for _, s := range m.streams {
			if s.hasNew {
				sum += int32(float64(s.frame[i]) * s.volume)
			}
		}
		// Clip to int16 range.
		if sum > 32767 {
			sum = 32767
		} else if sum < -32768 {
			sum = -32768
		}
		out[i] = int16(sum)
	}

	// Clear hasNew flags.
	for _, s := range m.streams {
		s.hasNew = false
	}

	return out
}

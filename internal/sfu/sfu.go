package sfu

import (
	"context"
	"sync"
	"time"
)

// Config holds SFU configuration.
type Config struct {
	GracePeriod time.Duration
	GCInterval  time.Duration
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		GracePeriod: 30 * time.Second,
		GCInterval:  10 * time.Second,
	}
}

// SFU manages rooms and WebRTC connections.
type SFU struct {
	config Config
	ctx    context.Context
	cancel context.CancelFunc

	mu    sync.RWMutex
	rooms map[string]*Room

	emptyAt map[string]time.Time
}

// New creates an SFU with default configuration.
func New() *SFU {
	return NewWithConfig(DefaultConfig())
}

// NewWithConfig creates an SFU with custom configuration.
func NewWithConfig(cfg Config) *SFU {
	ctx, cancel := context.WithCancel(context.Background())
	s := &SFU{
		config:  cfg,
		ctx:     ctx,
		cancel:  cancel,
		rooms:   make(map[string]*Room),
		emptyAt: make(map[string]time.Time),
	}
	go s.gcLoop()
	return s
}

// CreateRoom creates a new room and returns its code.
func (s *SFU) CreateRoom() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	var code string
	for {
		code = GenerateRoomCode()
		if _, exists := s.rooms[code]; !exists {
			break
		}
	}

	room := NewRoom(code)
	s.rooms[code] = room
	s.emptyAt[code] = time.Now()
	return code
}

// GetRoom returns a room by code.
func (s *SFU) GetRoom(code string) (*Room, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.rooms[code]
	return r, ok
}

// Close shuts down the SFU and all rooms.
func (s *SFU) Close() {
	s.cancel()
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.rooms {
		r.Close()
	}
}

func (s *SFU) gcLoop() {
	ticker := time.NewTicker(s.config.GCInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case now := <-ticker.C:
			s.mu.Lock()
			for code, room := range s.rooms {
				if room.IsEmpty() {
					if emptySince, ok := s.emptyAt[code]; ok {
						if now.Sub(emptySince) > s.config.GracePeriod {
							room.Close()
							delete(s.rooms, code)
							delete(s.emptyAt, code)
						}
					} else {
						s.emptyAt[code] = now
					}
				} else {
					delete(s.emptyAt, code)
				}
			}
			s.mu.Unlock()
		}
	}
}

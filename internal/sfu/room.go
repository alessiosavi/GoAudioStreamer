package sfu

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// Peer represents a user in a room.
type Peer struct {
	ID    string
	Name  string
	Muted bool
}

// Room is a voice session containing peers.
type Room struct {
	Code      string
	created   time.Time
	closed    chan struct{}
	closeOnce sync.Once

	mu    sync.RWMutex
	peers map[string]*Peer
}

// NewRoom creates a new room with the given code.
func NewRoom(code string) *Room {
	return &Room{
		Code:    code,
		created: time.Now(),
		closed:  make(chan struct{}),
		peers:   make(map[string]*Peer),
	}
}

// AddPeer creates a new peer with a generated ID and adds it to the room.
func (r *Room) AddPeer(name string) *Peer {
	r.mu.Lock()
	defer r.mu.Unlock()

	peer := &Peer{
		ID:   uuid.NewString(),
		Name: name,
	}
	r.peers[peer.ID] = peer
	return peer
}

// RemovePeer removes a peer by ID.
func (r *Room) RemovePeer(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.peers, id)
}

// GetPeer returns a peer by ID.
func (r *Room) GetPeer(id string) (*Peer, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.peers[id]
	return p, ok
}

// PeerList returns a snapshot of all peers.
func (r *Room) PeerList() []Peer {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]Peer, 0, len(r.peers))
	for _, p := range r.peers {
		list = append(list, *p)
	}
	return list
}

// PeerCount returns the number of peers.
func (r *Room) PeerCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.peers)
}

// IsEmpty returns true if the room has no peers.
func (r *Room) IsEmpty() bool {
	return r.PeerCount() == 0
}

// Close marks the room as closed.
func (r *Room) Close() {
	r.closeOnce.Do(func() {
		close(r.closed)
	})
}

// Done returns a channel that is closed when the room is closed.
func (r *Room) Done() <-chan struct{} {
	return r.closed
}

package sfu

import (
	"testing"
	"time"
)

func TestRoom_CreateAndJoin(t *testing.T) {
	room := NewRoom("TEST-CODE")

	if room.Code != "TEST-CODE" {
		t.Fatalf("code: got %q, want %q", room.Code, "TEST-CODE")
	}
	if room.PeerCount() != 0 {
		t.Fatalf("peer count: got %d, want 0", room.PeerCount())
	}

	peer := room.AddPeer("Alice")
	if peer.Name != "Alice" {
		t.Fatalf("name: got %q, want %q", peer.Name, "Alice")
	}
	if peer.ID == "" {
		t.Fatal("peer ID should not be empty")
	}
	if room.PeerCount() != 1 {
		t.Fatalf("peer count: got %d, want 1", room.PeerCount())
	}
}

func TestRoom_Leave(t *testing.T) {
	room := NewRoom("TEST-CODE")
	peer := room.AddPeer("Alice")

	room.RemovePeer(peer.ID)
	if room.PeerCount() != 0 {
		t.Fatalf("peer count after leave: got %d, want 0", room.PeerCount())
	}
}

func TestRoom_GetPeer(t *testing.T) {
	room := NewRoom("TEST-CODE")
	peer := room.AddPeer("Alice")

	got, ok := room.GetPeer(peer.ID)
	if !ok {
		t.Fatal("GetPeer should find Alice")
	}
	if got.Name != "Alice" {
		t.Fatalf("name: got %q, want %q", got.Name, "Alice")
	}

	_, ok = room.GetPeer("nonexistent")
	if ok {
		t.Fatal("GetPeer should not find nonexistent peer")
	}
}

func TestRoom_PeerList(t *testing.T) {
	room := NewRoom("TEST-CODE")
	room.AddPeer("Alice")
	room.AddPeer("Bob")

	peers := room.PeerList()
	if len(peers) != 2 {
		t.Fatalf("peer list: got %d, want 2", len(peers))
	}

	names := map[string]bool{}
	for _, p := range peers {
		names[p.Name] = true
	}
	if !names["Alice"] || !names["Bob"] {
		t.Fatalf("peer names: got %v", names)
	}
}

func TestRoom_IsEmpty(t *testing.T) {
	room := NewRoom("TEST-CODE")
	if !room.IsEmpty() {
		t.Fatal("new room should be empty")
	}
	peer := room.AddPeer("Alice")
	if room.IsEmpty() {
		t.Fatal("room with peer should not be empty")
	}
	room.RemovePeer(peer.ID)
	if !room.IsEmpty() {
		t.Fatal("room after last peer leaves should be empty")
	}
}

func TestRoom_Close(t *testing.T) {
	room := NewRoom("TEST-CODE")
	room.Close()

	select {
	case <-room.Done():
		// expected
	case <-time.After(time.Second):
		t.Fatal("Done channel should be closed after Close()")
	}
}

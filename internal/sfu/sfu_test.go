package sfu

import (
	"testing"
	"time"
)

func TestSFU_CreateRoom(t *testing.T) {
	s := New()
	defer s.Close()

	code := s.CreateRoom()
	if len(code) != 9 { // XXXX-XXXX
		t.Fatalf("room code length: got %d, want 9", len(code))
	}

	room, ok := s.GetRoom(code)
	if !ok {
		t.Fatal("room should exist after creation")
	}
	if room.Code != code {
		t.Fatalf("room code: got %q, want %q", room.Code, code)
	}
}

func TestSFU_GetRoom_NotFound(t *testing.T) {
	s := New()
	defer s.Close()

	_, ok := s.GetRoom("XXXX-XXXX")
	if ok {
		t.Fatal("should not find nonexistent room")
	}
}

func TestSFU_RoomGC(t *testing.T) {
	s := NewWithConfig(Config{GracePeriod: 50 * time.Millisecond, GCInterval: 20 * time.Millisecond})
	defer s.Close()

	code := s.CreateRoom()
	_, ok := s.GetRoom(code)
	if !ok {
		t.Fatal("room should exist")
	}

	// Room is empty from creation, so after grace period it should be GC'd.
	time.Sleep(150 * time.Millisecond)

	_, ok = s.GetRoom(code)
	if ok {
		t.Fatal("empty room should have been garbage collected")
	}
}

func TestSFU_RoomNotGCdWithPeers(t *testing.T) {
	s := NewWithConfig(Config{GracePeriod: 50 * time.Millisecond, GCInterval: 20 * time.Millisecond})
	defer s.Close()

	code := s.CreateRoom()
	room, _ := s.GetRoom(code)
	room.AddPeer("Alice")

	time.Sleep(150 * time.Millisecond)

	_, ok := s.GetRoom(code)
	if !ok {
		t.Fatal("room with peers should NOT be garbage collected")
	}
}

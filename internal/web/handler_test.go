package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"voxlink/internal/sfu"
)

func TestHandler_Health(t *testing.T) {
	s := sfu.New()
	defer s.Close()

	h := NewHandler(s, nil)
	req := httptest.NewRequest("GET", "/api/health", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "ok" {
		t.Fatalf("status: got %q, want %q", resp["status"], "ok")
	}
}

func TestHandler_RoomInfo_Exists(t *testing.T) {
	s := sfu.New()
	defer s.Close()

	code := s.CreateRoom()
	room, _ := s.GetRoom(code)
	room.AddPeer("Alice")

	h := NewHandler(s, nil)
	req := httptest.NewRequest("GET", "/api/room/"+code, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", w.Code, http.StatusOK)
	}

	var resp struct {
		Exists bool `json:"exists"`
		Peers  int  `json:"peers"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if !resp.Exists {
		t.Fatal("room should exist")
	}
	if resp.Peers != 1 {
		t.Fatalf("peers: got %d, want 1", resp.Peers)
	}
}

func TestHandler_RoomInfo_NotFound(t *testing.T) {
	s := sfu.New()
	defer s.Close()

	h := NewHandler(s, nil)
	req := httptest.NewRequest("GET", "/api/room/NOPE-NOPE", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", w.Code, http.StatusOK)
	}

	var resp struct {
		Exists bool `json:"exists"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Exists {
		t.Fatal("room should not exist")
	}
}

func TestHandler_StaticFiles(t *testing.T) {
	s := sfu.New()
	defer s.Close()

	h := NewHandler(s, nil)
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", w.Code, http.StatusOK)
	}
}

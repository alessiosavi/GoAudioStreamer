package signaling

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"voxlink/internal/sfu"
)

func TestServer_CreateRoom(t *testing.T) {
	s := sfu.New()
	defer s.Close()

	srv := httptest.NewServer(NewHandler(s, nil))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, "ws"+srv.URL[4:]+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()

	env, _ := NewEnvelope(MsgCreateRoom, CreateRoomPayload{Name: "Alice"})
	if err := wsjson.Write(ctx, conn, env); err != nil {
		t.Fatal(err)
	}

	var resp Envelope
	if err := wsjson.Read(ctx, conn, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Type != MsgRoomCreated {
		t.Fatalf("type: got %q, want %q", resp.Type, MsgRoomCreated)
	}

	var payload RoomCreatedPayload
	if err := json.Unmarshal(resp.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Code) != 9 {
		t.Fatalf("room code length: got %d, want 9", len(payload.Code))
	}
	if payload.PeerID == "" {
		t.Fatal("peerID should not be empty")
	}
}

func TestServer_JoinRoom(t *testing.T) {
	s := sfu.New()
	defer s.Close()

	srv := httptest.NewServer(NewHandler(s, nil))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	alice, _, _ := websocket.Dial(ctx, "ws"+srv.URL[4:]+"/ws", nil)
	defer alice.CloseNow()

	env, _ := NewEnvelope(MsgCreateRoom, CreateRoomPayload{Name: "Alice"})
	wsjson.Write(ctx, alice, env)

	var aliceResp Envelope
	wsjson.Read(ctx, alice, &aliceResp)
	var created RoomCreatedPayload
	json.Unmarshal(aliceResp.Payload, &created)

	bob, _, _ := websocket.Dial(ctx, "ws"+srv.URL[4:]+"/ws", nil)
	defer bob.CloseNow()

	joinEnv, _ := NewEnvelope(MsgJoinRoom, JoinRoomPayload{Code: created.Code, Name: "Bob"})
	wsjson.Write(ctx, bob, joinEnv)

	var bobResp Envelope
	wsjson.Read(ctx, bob, &bobResp)
	if bobResp.Type != MsgRoomJoined {
		t.Fatalf("type: got %q, want %q", bobResp.Type, MsgRoomJoined)
	}

	var joined RoomJoinedPayload
	json.Unmarshal(bobResp.Payload, &joined)
	if len(joined.Peers) != 1 {
		t.Fatalf("peers: got %d, want 1 (Alice)", len(joined.Peers))
	}
	if joined.Peers[0].Name != "Alice" {
		t.Fatalf("peer name: got %q, want %q", joined.Peers[0].Name, "Alice")
	}

	var aliceNotif Envelope
	wsjson.Read(ctx, alice, &aliceNotif)
	if aliceNotif.Type != MsgPeerJoined {
		t.Fatalf("alice notification: got %q, want %q", aliceNotif.Type, MsgPeerJoined)
	}
}

func TestServer_JoinNonexistentRoom(t *testing.T) {
	s := sfu.New()
	defer s.Close()

	srv := httptest.NewServer(NewHandler(s, nil))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, _, _ := websocket.Dial(ctx, "ws"+srv.URL[4:]+"/ws", nil)
	defer conn.CloseNow()

	env, _ := NewEnvelope(MsgJoinRoom, JoinRoomPayload{Code: "NOPE-NOPE", Name: "Bob"})
	wsjson.Write(ctx, conn, env)

	var resp Envelope
	wsjson.Read(ctx, conn, &resp)
	if resp.Type != MsgError {
		t.Fatalf("type: got %q, want %q", resp.Type, MsgError)
	}
}

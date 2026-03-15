package test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"voxlink/internal/sfu"
	"voxlink/internal/signaling"
	"voxlink/internal/web"
)

func TestIntegration_FullRoomLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	sfuEngine := sfu.New()
	defer sfuEngine.Close()

	mux := http.NewServeMux()
	mux.Handle("/ws", signaling.NewHandler(sfuEngine))
	mux.Handle("/", web.NewHandler(sfuEngine, nil))

	srv := httptest.NewServer(mux)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	wsURL := "ws" + srv.URL[4:] + "/ws"

	// 1. Alice creates a room.
	alice, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer alice.CloseNow()

	createEnv, _ := signaling.NewEnvelope(signaling.MsgCreateRoom, signaling.CreateRoomPayload{Name: "Alice"})
	wsjson.Write(ctx, alice, createEnv)

	var aliceResp signaling.Envelope
	wsjson.Read(ctx, alice, &aliceResp)
	if aliceResp.Type != signaling.MsgRoomCreated {
		t.Fatalf("expected room-created, got %s", aliceResp.Type)
	}

	var created signaling.RoomCreatedPayload
	json.Unmarshal(aliceResp.Payload, &created)
	t.Logf("Room created: %s", created.Code)

	// 2. Verify room via REST.
	resp, _ := http.Get(srv.URL + "/api/room/" + created.Code)
	var roomInfo struct {
		Exists bool `json:"exists"`
		Peers  int  `json:"peers"`
	}
	json.NewDecoder(resp.Body).Decode(&roomInfo)
	resp.Body.Close()
	if !roomInfo.Exists || roomInfo.Peers != 1 {
		t.Fatalf("room: exists=%v peers=%d", roomInfo.Exists, roomInfo.Peers)
	}

	// 3. Bob joins.
	bob, _, _ := websocket.Dial(ctx, wsURL, nil)
	defer bob.CloseNow()

	joinEnv, _ := signaling.NewEnvelope(signaling.MsgJoinRoom, signaling.JoinRoomPayload{Code: created.Code, Name: "Bob"})
	wsjson.Write(ctx, bob, joinEnv)

	var bobResp signaling.Envelope
	wsjson.Read(ctx, bob, &bobResp)
	if bobResp.Type != signaling.MsgRoomJoined {
		t.Fatalf("expected room-joined, got %s", bobResp.Type)
	}

	var joined signaling.RoomJoinedPayload
	json.Unmarshal(bobResp.Payload, &joined)
	if len(joined.Peers) != 1 || joined.Peers[0].Name != "Alice" {
		t.Fatalf("Bob should see Alice, got: %+v", joined.Peers)
	}

	// 4. Alice gets peer-joined.
	var aliceNotif signaling.Envelope
	wsjson.Read(ctx, alice, &aliceNotif)
	if aliceNotif.Type != signaling.MsgPeerJoined {
		t.Fatalf("expected peer-joined, got %s", aliceNotif.Type)
	}

	// 5. Bob mutes.
	muteEnv, _ := signaling.NewEnvelope(signaling.MsgMute, signaling.MutePayload{Muted: true})
	wsjson.Write(ctx, bob, muteEnv)

	var muteNotif signaling.Envelope
	wsjson.Read(ctx, alice, &muteNotif)
	if muteNotif.Type != signaling.MsgPeerMuted {
		t.Fatalf("expected peer-muted, got %s", muteNotif.Type)
	}

	// 6. Bob leaves.
	leaveEnv, _ := signaling.NewEnvelope(signaling.MsgLeave, signaling.LeavePayload{})
	wsjson.Write(ctx, bob, leaveEnv)

	var leftNotif signaling.Envelope
	wsjson.Read(ctx, alice, &leftNotif)
	if leftNotif.Type != signaling.MsgPeerLeft {
		t.Fatalf("expected peer-left, got %s", leftNotif.Type)
	}

	// 7. Health check.
	healthResp, _ := http.Get(srv.URL + "/api/health")
	var health map[string]string
	json.NewDecoder(healthResp.Body).Decode(&health)
	healthResp.Body.Close()
	if health["status"] != "ok" {
		t.Fatalf("health: %v", health)
	}

	t.Log("Full room lifecycle test passed!")
}

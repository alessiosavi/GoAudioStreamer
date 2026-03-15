package signaling

import (
	"encoding/json"
	"testing"
)

func TestEnvelopeMarshalCreateRoom(t *testing.T) {
	env := Envelope{
		Type:    MsgCreateRoom,
		Payload: mustMarshal(t, CreateRoomPayload{Name: "Alice"}),
	}
	data, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Envelope
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Type != MsgCreateRoom {
		t.Errorf("got type %q, want %q", decoded.Type, MsgCreateRoom)
	}
	var payload CreateRoomPayload
	if err := json.Unmarshal(decoded.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Name != "Alice" {
		t.Errorf("got name %q, want %q", payload.Name, "Alice")
	}
}

func TestEnvelopeMarshalRoomCreated(t *testing.T) {
	env := Envelope{
		Type: MsgRoomCreated,
		Payload: mustMarshal(t, RoomCreatedPayload{
			Code:   "VOXL-A3F7",
			PeerID: "uuid-123",
			Peers:  []PeerInfo{},
		}),
	}
	data, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Envelope
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	var payload RoomCreatedPayload
	if err := json.Unmarshal(decoded.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Code != "VOXL-A3F7" {
		t.Errorf("got code %q, want %q", payload.Code, "VOXL-A3F7")
	}
	if payload.PeerID != "uuid-123" {
		t.Errorf("got peerID %q, want %q", payload.PeerID, "uuid-123")
	}
}

func TestAllMessageTypesRoundTrip(t *testing.T) {
	cases := []struct {
		name    string
		msgType string
		payload any
	}{
		{"create-room", MsgCreateRoom, CreateRoomPayload{Name: "Alice"}},
		{"join-room", MsgJoinRoom, JoinRoomPayload{Code: "ABCD-1234", Name: "Bob"}},
		{"answer", MsgAnswer, AnswerPayload{SDP: "v=0\r\n..."}},
		{"ice-candidate", MsgICECandidate, ICECandidatePayload{Candidate: "candidate:1 1 udp ..."}},
		{"rejoin", MsgRejoin, RejoinPayload{Code: "ABCD-1234", PeerID: "uuid-1"}},
		{"leave", MsgLeave, LeavePayload{}},
		{"mute", MsgMute, MutePayload{Muted: true}},
		{"room-created", MsgRoomCreated, RoomCreatedPayload{Code: "ABCD-1234", PeerID: "uuid-1", Peers: []PeerInfo{}}},
		{"room-joined", MsgRoomJoined, RoomJoinedPayload{Code: "ABCD-1234", PeerID: "uuid-2", Peers: []PeerInfo{{ID: "uuid-1", Name: "Alice"}}}},
		{"peer-joined", MsgPeerJoined, PeerJoinedPayload{ID: "uuid-2", Name: "Bob"}},
		{"peer-left", MsgPeerLeft, PeerLeftPayload{ID: "uuid-2"}},
		{"offer", MsgOffer, OfferPayload{SDP: "v=0\r\n..."}},
		{"peer-muted", MsgPeerMuted, PeerMutedPayload{ID: "uuid-1", Muted: true}},
		{"error", MsgError, ErrorPayload{Message: "room not found"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := Envelope{
				Type:    tc.msgType,
				Payload: mustMarshal(t, tc.payload),
			}
			data, err := json.Marshal(env)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var decoded Envelope
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if decoded.Type != tc.msgType {
				t.Errorf("type: got %q, want %q", decoded.Type, tc.msgType)
			}
		})
	}
}

func mustMarshal(t *testing.T, v any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

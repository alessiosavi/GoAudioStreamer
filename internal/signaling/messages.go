package signaling

import "encoding/json"

const (
	MsgCreateRoom   = "create-room"
	MsgJoinRoom     = "join-room"
	MsgAnswer       = "answer"
	MsgICECandidate = "ice-candidate"
	MsgRejoin       = "rejoin"
	MsgLeave        = "leave"
	MsgMute         = "mute"
	MsgRoomCreated  = "room-created"
	MsgRoomJoined   = "room-joined"
	MsgPeerJoined   = "peer-joined"
	MsgPeerLeft     = "peer-left"
	MsgOffer        = "offer"
	MsgPeerMuted    = "peer-muted"
	MsgError        = "error"
)

type Envelope struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type PeerInfo struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Muted bool   `json:"muted,omitempty"`
}

type CreateRoomPayload struct {
	Name string `json:"name"`
}

type JoinRoomPayload struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

type AnswerPayload struct {
	SDP string `json:"sdp"`
}

type ICECandidatePayload struct {
	Candidate string `json:"candidate"`
}

type RejoinPayload struct {
	Code   string `json:"code"`
	PeerID string `json:"peerId"`
}

type LeavePayload struct{}

type MutePayload struct {
	Muted bool `json:"muted"`
}

type RoomCreatedPayload struct {
	Code   string     `json:"code"`
	PeerID string     `json:"peerId"`
	Peers  []PeerInfo `json:"peers"`
}

type RoomJoinedPayload struct {
	Code   string     `json:"code"`
	PeerID string     `json:"peerId"`
	Peers  []PeerInfo `json:"peers"`
}

type PeerJoinedPayload struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type PeerLeftPayload struct {
	ID string `json:"id"`
}

type OfferPayload struct {
	SDP string `json:"sdp"`
}

type PeerMutedPayload struct {
	ID    string `json:"id"`
	Muted bool   `json:"muted"`
}

type ErrorPayload struct {
	Message string `json:"message"`
}

func NewEnvelope(msgType string, payload any) (Envelope, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return Envelope{}, err
	}
	return Envelope{Type: msgType, Payload: data}, nil
}

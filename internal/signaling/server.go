package signaling

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"voxlink/internal/sfu"
)

type clientConn struct {
	conn     *websocket.Conn
	peerID   string
	roomCode string
	mu       sync.Mutex
}

func (c *clientConn) send(ctx context.Context, env Envelope) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return wsjson.Write(ctx, c.conn, env)
}

// Handler manages signaling state.
type Handler struct {
	sfu    *sfu.SFU
	logger *slog.Logger

	mu      sync.RWMutex
	clients map[string]*clientConn
}

// NewHandler creates a signaling handler backed by the given SFU.
func NewHandler(s *sfu.SFU) http.Handler {
	h := &Handler{
		sfu:     s,
		logger:  slog.Default(),
		clients: make(map[string]*clientConn),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", h.handleWS)
	return mux
}

func (h *Handler) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		h.logger.Error("websocket accept", "err", err)
		return
	}
	defer conn.CloseNow()

	conn.SetReadLimit(65536)
	ctx := r.Context()
	client := &clientConn{conn: conn}

	for {
		var env Envelope
		if err := wsjson.Read(ctx, conn, &env); err != nil {
			if client.peerID != "" {
				h.handleDisconnect(ctx, client)
			}
			return
		}

		switch env.Type {
		case MsgCreateRoom:
			h.handleCreateRoom(ctx, client, env.Payload)
		case MsgJoinRoom:
			h.handleJoinRoom(ctx, client, env.Payload)
		case MsgAnswer:
			h.handleAnswer(ctx, client, env.Payload)
		case MsgICECandidate:
			h.handleICECandidate(ctx, client, env.Payload)
		case MsgRejoin:
			h.handleRejoin(ctx, client, env.Payload)
		case MsgLeave:
			h.handleLeave(ctx, client)
		case MsgMute:
			h.handleMute(ctx, client, env.Payload)
		default:
			h.sendError(ctx, client, "unknown message type: "+env.Type)
		}
	}
}

func (h *Handler) handleCreateRoom(ctx context.Context, client *clientConn, payload json.RawMessage) {
	var msg CreateRoomPayload
	if err := json.Unmarshal(payload, &msg); err != nil {
		h.sendError(ctx, client, "invalid create-room payload")
		return
	}

	code := h.sfu.CreateRoom()
	room, _ := h.sfu.GetRoom(code)
	peer := room.AddPeer(msg.Name)

	client.peerID = peer.ID
	client.roomCode = code

	h.mu.Lock()
	h.clients[peer.ID] = client
	h.mu.Unlock()

	env, _ := NewEnvelope(MsgRoomCreated, RoomCreatedPayload{
		Code:   code,
		PeerID: peer.ID,
		Peers:  []PeerInfo{},
	})
	client.send(ctx, env)
}

func (h *Handler) handleJoinRoom(ctx context.Context, client *clientConn, payload json.RawMessage) {
	var msg JoinRoomPayload
	if err := json.Unmarshal(payload, &msg); err != nil {
		h.sendError(ctx, client, "invalid join-room payload")
		return
	}

	room, ok := h.sfu.GetRoom(msg.Code)
	if !ok {
		h.sendError(ctx, client, "room not found")
		return
	}

	existingPeers := room.PeerList()
	peer := room.AddPeer(msg.Name)
	client.peerID = peer.ID
	client.roomCode = msg.Code

	h.mu.Lock()
	h.clients[peer.ID] = client
	h.mu.Unlock()

	peerInfos := make([]PeerInfo, 0, len(existingPeers))
	for _, p := range existingPeers {
		peerInfos = append(peerInfos, PeerInfo{ID: p.ID, Name: p.Name, Muted: p.Muted})
	}

	joinedEnv, _ := NewEnvelope(MsgRoomJoined, RoomJoinedPayload{
		Code:   msg.Code,
		PeerID: peer.ID,
		Peers:  peerInfos,
	})
	client.send(ctx, joinedEnv)

	notifEnv, _ := NewEnvelope(MsgPeerJoined, PeerJoinedPayload{
		ID:   peer.ID,
		Name: peer.Name,
	})
	h.broadcastToRoom(ctx, msg.Code, peer.ID, notifEnv)
}

func (h *Handler) handleLeave(ctx context.Context, client *clientConn) {
	h.handleDisconnect(ctx, client)
}

func (h *Handler) handleMute(ctx context.Context, client *clientConn, payload json.RawMessage) {
	var msg MutePayload
	if err := json.Unmarshal(payload, &msg); err != nil {
		return
	}
	if client.roomCode == "" {
		return
	}
	room, ok := h.sfu.GetRoom(client.roomCode)
	if !ok {
		return
	}
	if peer, ok := room.GetPeer(client.peerID); ok {
		peer.Muted = msg.Muted
	}
	env, _ := NewEnvelope(MsgPeerMuted, PeerMutedPayload{ID: client.peerID, Muted: msg.Muted})
	h.broadcastToRoom(ctx, client.roomCode, client.peerID, env)
}

func (h *Handler) handleAnswer(ctx context.Context, client *clientConn, payload json.RawMessage) {
	var msg AnswerPayload
	if err := json.Unmarshal(payload, &msg); err != nil {
		h.sendError(ctx, client, "invalid answer payload")
		return
	}
	// TODO(webrtc): Set remote description on the client's SFU-side PeerConnection.
	// This will be wired when PeerManager is integrated with signaling.
	h.logger.Info("received SDP answer", "peer", client.peerID)
}

func (h *Handler) handleICECandidate(ctx context.Context, client *clientConn, payload json.RawMessage) {
	var msg ICECandidatePayload
	if err := json.Unmarshal(payload, &msg); err != nil {
		h.sendError(ctx, client, "invalid ice-candidate payload")
		return
	}
	// TODO(webrtc): Add ICE candidate to the client's SFU-side PeerConnection.
	h.logger.Info("received ICE candidate", "peer", client.peerID)
}

func (h *Handler) handleRejoin(ctx context.Context, client *clientConn, payload json.RawMessage) {
	var msg RejoinPayload
	if err := json.Unmarshal(payload, &msg); err != nil {
		h.sendError(ctx, client, "invalid rejoin payload")
		return
	}
	room, ok := h.sfu.GetRoom(msg.Code)
	if !ok {
		h.sendError(ctx, client, "room not found")
		return
	}
	peer, ok := room.GetPeer(msg.PeerID)
	if !ok {
		h.sendError(ctx, client, "peer not found or grace period expired")
		return
	}
	client.peerID = peer.ID
	client.roomCode = msg.Code
	h.mu.Lock()
	h.clients[peer.ID] = client
	h.mu.Unlock()
	env, _ := NewEnvelope(MsgRoomJoined, RoomJoinedPayload{
		Code: msg.Code, PeerID: peer.ID, Peers: toPeerInfoList(room.PeerList(), peer.ID),
	})
	client.send(ctx, env)
}

func toPeerInfoList(peers []sfu.Peer, excludeID string) []PeerInfo {
	out := make([]PeerInfo, 0, len(peers))
	for _, p := range peers {
		if p.ID == excludeID {
			continue
		}
		out = append(out, PeerInfo{ID: p.ID, Name: p.Name, Muted: p.Muted})
	}
	return out
}

func (h *Handler) handleDisconnect(ctx context.Context, client *clientConn) {
	if client.roomCode == "" || client.peerID == "" {
		return
	}
	room, ok := h.sfu.GetRoom(client.roomCode)
	if ok {
		room.RemovePeer(client.peerID)
	}

	h.mu.Lock()
	delete(h.clients, client.peerID)
	h.mu.Unlock()

	env, _ := NewEnvelope(MsgPeerLeft, PeerLeftPayload{ID: client.peerID})
	h.broadcastToRoom(ctx, client.roomCode, client.peerID, env)

	client.peerID = ""
	client.roomCode = ""
}

func (h *Handler) broadcastToRoom(ctx context.Context, roomCode, excludeID string, env Envelope) {
	room, ok := h.sfu.GetRoom(roomCode)
	if !ok {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, peer := range room.PeerList() {
		if peer.ID == excludeID {
			continue
		}
		if c, ok := h.clients[peer.ID]; ok {
			go c.send(ctx, env)
		}
	}
}

func (h *Handler) sendError(ctx context.Context, client *clientConn, msg string) {
	env, _ := NewEnvelope(MsgError, ErrorPayload{Message: msg})
	client.send(ctx, env)
}

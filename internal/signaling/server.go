package signaling

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/pion/webrtc/v4"
	"go.uber.org/zap"

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
	sfu         *sfu.SFU
	peerManager *sfu.PeerManager
	logger      *zap.Logger

	mu          sync.RWMutex
	clients     map[string]*clientConn
	webrtcPeers map[string]*sfu.WebRTCPeer // peerID → WebRTCPeer
}

// NewHandler creates a signaling handler backed by the given SFU.
// peerManager may be nil (e.g. in tests) — WebRTC setup is skipped when nil.
func NewHandler(s *sfu.SFU, logger *zap.Logger, opts ...HandlerOption) http.Handler {
	if logger == nil {
		logger, _ = zap.NewDevelopment()
	}
	h := &Handler{
		sfu:         s,
		logger:      logger,
		clients:     make(map[string]*clientConn),
		webrtcPeers: make(map[string]*sfu.WebRTCPeer),
	}
	for _, opt := range opts {
		opt(h)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", h.handleWS)
	return mux
}

// HandlerOption configures optional Handler fields.
type HandlerOption func(*Handler)

// WithPeerManager sets the PeerManager for WebRTC support.
func WithPeerManager(pm *sfu.PeerManager) HandlerOption {
	return func(h *Handler) {
		h.peerManager = pm
	}
}

func (h *Handler) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		h.logger.Error("websocket accept", zap.Error(err))
		return
	}
	defer conn.CloseNow()

	conn.SetReadLimit(65536)
	ctx := r.Context()
	client := &clientConn{conn: conn}

	h.logger.Info("client connected", zap.String("remote", r.RemoteAddr))

	for {
		var env Envelope
		if err := wsjson.Read(ctx, conn, &env); err != nil {
			if client.peerID != "" {
				h.logger.Info("client disconnected", zap.String("peer", client.peerID))
				h.handleDisconnect(ctx, client)
			} else {
				h.logger.Debug("anonymous client disconnected")
			}
			return
		}

		h.logger.Debug("recv", zap.String("type", env.Type), zap.String("peer", client.peerID))

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

	h.logger.Info("room created", zap.String("code", code), zap.String("peer", peer.ID), zap.String("name", msg.Name))

	env, _ := NewEnvelope(MsgRoomCreated, RoomCreatedPayload{
		Code:   code,
		PeerID: peer.ID,
		Peers:  []PeerInfo{},
	})
	client.send(ctx, env)

	// Set up WebRTC PeerConnection for the new peer.
	h.setupPeerConnection(ctx, client, peer, code)
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

	h.logger.Info("peer joined", zap.String("room", msg.Code), zap.String("peer", peer.ID), zap.String("name", msg.Name))

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

	// Set up WebRTC PeerConnection for the joining peer.
	h.setupPeerConnection(ctx, client, peer, msg.Code)
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

	h.mu.RLock()
	wp, ok := h.webrtcPeers[client.peerID]
	h.mu.RUnlock()
	if !ok {
		h.logger.Warn("answer for unknown webrtc peer", zap.String("peer", client.peerID))
		return
	}

	wp.Mu.Lock()
	defer wp.Mu.Unlock()

	err := wp.PC.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  msg.SDP,
	})
	if err != nil {
		h.logger.Error("set remote description", zap.String("peer", client.peerID), zap.Error(err))
		h.sendError(ctx, client, "failed to set answer")
		return
	}

	// Drain any ICE candidates that arrived before the remote description was set.
	for _, c := range wp.PendingCandidates {
		if err := wp.PC.AddICECandidate(c); err != nil {
			h.logger.Warn("add buffered ICE candidate", zap.String("peer", client.peerID), zap.Error(err))
		}
	}
	wp.PendingCandidates = nil

	h.logger.Info("set SDP answer", zap.String("peer", client.peerID))
}

func (h *Handler) handleICECandidate(ctx context.Context, client *clientConn, payload json.RawMessage) {
	var msg ICECandidatePayload
	if err := json.Unmarshal(payload, &msg); err != nil {
		h.sendError(ctx, client, "invalid ice-candidate payload")
		return
	}

	h.mu.RLock()
	wp, ok := h.webrtcPeers[client.peerID]
	h.mu.RUnlock()
	if !ok {
		h.logger.Warn("ICE candidate for unknown webrtc peer", zap.String("peer", client.peerID))
		return
	}

	candidate := webrtc.ICECandidateInit{Candidate: msg.Candidate}

	wp.Mu.Lock()
	defer wp.Mu.Unlock()

	// If remote description is not yet set, buffer the candidate.
	if wp.PC.RemoteDescription() == nil {
		wp.PendingCandidates = append(wp.PendingCandidates, candidate)
		h.logger.Debug("buffered ICE candidate (no remote desc yet)", zap.String("peer", client.peerID))
		return
	}

	if err := wp.PC.AddICECandidate(candidate); err != nil {
		h.logger.Warn("add ICE candidate", zap.String("peer", client.peerID), zap.Error(err))
		return
	}

	h.logger.Debug("added ICE candidate", zap.String("peer", client.peerID))
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

	// Set up a fresh WebRTC PeerConnection for the rejoining peer.
	h.setupPeerConnection(ctx, client, peer, msg.Code)
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

	peerID := client.peerID
	roomCode := client.roomCode

	// Close WebRTC PeerConnection and cancel all subscriptions.
	h.mu.Lock()
	wp, hasWP := h.webrtcPeers[peerID]
	delete(h.webrtcPeers, peerID)
	delete(h.clients, peerID)
	h.mu.Unlock()

	if hasWP {
		wp.Mu.Lock()
		for srcID, sub := range wp.Subs {
			sub.Cancel()
			delete(wp.Subs, srcID)
		}
		wp.PC.Close()
		wp.Mu.Unlock()
		h.logger.Info("closed webrtc peer connection", zap.String("peer", peerID))

		// Also remove subscriptions to this peer from all other peers in the room.
		h.removeSubscriptionsForPeer(peerID, roomCode)
	}

	room, ok := h.sfu.GetRoom(roomCode)
	if ok {
		room.RemovePeer(peerID)
	}

	env, _ := NewEnvelope(MsgPeerLeft, PeerLeftPayload{ID: peerID})
	h.broadcastToRoom(ctx, roomCode, peerID, env)

	client.peerID = ""
	client.roomCode = ""
}

// setupPeerConnection creates a WebRTC PeerConnection for a peer, wires
// OnTrack / OnICECandidate callbacks, and sends the initial SDP offer.
// If peerManager is nil (e.g. in tests), this is a no-op.
func (h *Handler) setupPeerConnection(ctx context.Context, client *clientConn, peer *sfu.Peer, roomCode string) {
	if h.peerManager == nil {
		return
	}

	pc, offer, err := h.peerManager.CreatePeerConnection()
	if err != nil {
		h.logger.Error("create peer connection", zap.String("peer", peer.ID), zap.Error(err))
		h.sendError(ctx, client, "failed to create WebRTC connection")
		return
	}

	wp := &sfu.WebRTCPeer{
		Peer: peer,
		PC:   pc,
		Subs: make(map[string]*sfu.Subscription),
	}

	h.mu.Lock()
	h.webrtcPeers[peer.ID] = wp
	h.mu.Unlock()

	peerID := peer.ID

	// Log ICE and PeerConnection state changes for diagnostics.
	pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		h.logger.Info("ICE state changed", zap.String("peer", peerID), zap.String("state", state.String()))
	})
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		h.logger.Info("PC state changed", zap.String("peer", peerID), zap.String("state", state.String()))
	})

	// OnICECandidate: send trickle ICE candidates to the client.
	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return // gathering complete
		}
		env, err := NewEnvelope(MsgICECandidate, ICECandidatePayload{
			Candidate: c.ToJSON().Candidate,
		})
		if err != nil {
			h.logger.Error("marshal ICE candidate", zap.Error(err))
			return
		}
		if err := client.send(ctx, env); err != nil {
			h.logger.Warn("send ICE candidate", zap.String("peer", peerID), zap.Error(err))
		}
	})

	// OnTrack: when the client sends audio, forward it to all other peers in the room.
	pc.OnTrack(func(remoteTrack *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		h.logger.Info("received track from peer",
			zap.String("peer", peerID),
			zap.String("track", remoteTrack.ID()),
			zap.String("codec", remoteTrack.Codec().MimeType),
		)

		// Forward this track to every other WebRTC peer in the same room.
		h.subscribeRoomPeers(ctx, peerID, roomCode, remoteTrack)
	})

	// Send the SDP offer to the client.
	h.sendOffer(ctx, client, peerID, offer)
}

// sendOffer sends an SDP offer to a client via WebSocket.
func (h *Handler) sendOffer(ctx context.Context, client *clientConn, peerID string, offer webrtc.SessionDescription) {
	env, err := NewEnvelope(MsgOffer, OfferPayload{SDP: offer.SDP})
	if err != nil {
		h.logger.Error("marshal offer", zap.String("peer", peerID), zap.Error(err))
		return
	}
	if err := client.send(ctx, env); err != nil {
		h.logger.Error("send offer", zap.String("peer", peerID), zap.Error(err))
	}
}

// subscribeRoomPeers forwards a remote track from sourcePeerID to all other
// WebRTC peers in the given room. Each subscription triggers renegotiation
// (a new SDP offer) for the subscriber.
func (h *Handler) subscribeRoomPeers(ctx context.Context, sourcePeerID, roomCode string, remoteTrack *webrtc.TrackRemote) {
	room, ok := h.sfu.GetRoom(roomCode)
	if !ok {
		return
	}

	for _, peer := range room.PeerList() {
		if peer.ID == sourcePeerID {
			continue
		}

		subscriberID := peer.ID

		h.mu.RLock()
		subWP, ok := h.webrtcPeers[subscriberID]
		subClient, hasClient := h.clients[subscriberID]
		h.mu.RUnlock()
		if !ok || !hasClient {
			continue
		}

		subWP.Mu.Lock()
		// Avoid duplicate subscriptions from the same source.
		if _, exists := subWP.Subs[sourcePeerID]; exists {
			subWP.Mu.Unlock()
			continue
		}

		sub, err := h.peerManager.SubscribeToTrack(ctx, remoteTrack, subWP.PC)
		if err != nil {
			subWP.Mu.Unlock()
			h.logger.Error("subscribe to track",
				zap.String("source", sourcePeerID),
				zap.String("subscriber", subscriberID),
				zap.Error(err),
			)
			continue
		}
		subWP.Subs[sourcePeerID] = sub
		subWP.Mu.Unlock()

		h.logger.Info("subscribed peer to track",
			zap.String("source", sourcePeerID),
			zap.String("subscriber", subscriberID),
		)

		// Renegotiate: create a new offer for the subscriber since we added a track.
		h.renegotiate(ctx, subClient, subWP)
	}
}

// renegotiate creates a new SDP offer for a WebRTC peer (after adding a track)
// and sends it to the client. The client must respond with an answer.
func (h *Handler) renegotiate(ctx context.Context, client *clientConn, wp *sfu.WebRTCPeer) {
	wp.Mu.Lock()
	defer wp.Mu.Unlock()

	offer, err := wp.PC.CreateOffer(nil)
	if err != nil {
		h.logger.Error("renegotiate create offer", zap.String("peer", wp.ID), zap.Error(err))
		return
	}
	if err := wp.PC.SetLocalDescription(offer); err != nil {
		h.logger.Error("renegotiate set local desc", zap.String("peer", wp.ID), zap.Error(err))
		return
	}

	// Clear pending candidates for the new negotiation round.
	wp.PendingCandidates = nil

	h.sendOffer(ctx, client, wp.ID, offer)
}

// removeSubscriptionsForPeer cancels subscriptions to the given peer
// from all other peers in the room.
func (h *Handler) removeSubscriptionsForPeer(peerID, roomCode string) {
	room, ok := h.sfu.GetRoom(roomCode)
	if !ok {
		return
	}

	for _, peer := range room.PeerList() {
		if peer.ID == peerID {
			continue
		}

		h.mu.RLock()
		wp, ok := h.webrtcPeers[peer.ID]
		h.mu.RUnlock()
		if !ok {
			continue
		}

		wp.Mu.Lock()
		if sub, exists := wp.Subs[peerID]; exists {
			sub.Cancel()
			delete(wp.Subs, peerID)
		}
		wp.Mu.Unlock()
	}
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

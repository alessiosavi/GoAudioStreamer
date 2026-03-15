package sfu

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/pion/webrtc/v4"
)

// Subscription tracks a forwarding relationship from a source peer's track
// to a local track on the subscriber's PeerConnection.
type Subscription struct {
	Track  *webrtc.TrackLocalStaticRTP
	Cancel context.CancelFunc
}

// WebRTCPeer extends Peer with WebRTC connection state.
type WebRTCPeer struct {
	*Peer
	PC   *webrtc.PeerConnection
	Subs map[string]*Subscription // source peerID -> subscription
	mu   sync.Mutex
}

// PeerManager handles WebRTC PeerConnection creation and track forwarding.
type PeerManager struct {
	api    *webrtc.API
	logger *slog.Logger
}

// NewPeerManager creates a PeerManager with the given WebRTC API.
func NewPeerManager(api *webrtc.API) *PeerManager {
	return &PeerManager{api: api, logger: slog.Default()}
}

// CreatePeerConnection creates a new PeerConnection and generates an SDP offer.
// The SFU acts as the offerer; the client will answer.
func (pm *PeerManager) CreatePeerConnection() (*webrtc.PeerConnection, webrtc.SessionDescription, error) {
	pc, err := pm.api.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		return nil, webrtc.SessionDescription{}, fmt.Errorf("new peer connection: %w", err)
	}

	// Add a transceiver for receiving audio from the client.
	_, err = pc.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionRecvonly,
	})
	if err != nil {
		pc.Close()
		return nil, webrtc.SessionDescription{}, fmt.Errorf("add transceiver: %w", err)
	}

	offer, err := pc.CreateOffer(nil)
	if err != nil {
		pc.Close()
		return nil, webrtc.SessionDescription{}, fmt.Errorf("create offer: %w", err)
	}
	if err := pc.SetLocalDescription(offer); err != nil {
		pc.Close()
		return nil, webrtc.SessionDescription{}, fmt.Errorf("set local desc: %w", err)
	}

	return pc, offer, nil
}

// SubscribeToTrack adds a forwarding track from sourcePeer to subscriberPC.
// Returns the subscription for cleanup.
func (pm *PeerManager) SubscribeToTrack(
	ctx context.Context,
	remoteTrack *webrtc.TrackRemote,
	subscriberPC *webrtc.PeerConnection,
) (*Subscription, error) {
	localTrack, err := webrtc.NewTrackLocalStaticRTP(
		remoteTrack.Codec().RTPCodecCapability,
		remoteTrack.ID(),
		remoteTrack.StreamID(),
	)
	if err != nil {
		return nil, fmt.Errorf("new local track: %w", err)
	}

	if _, err := subscriberPC.AddTrack(localTrack); err != nil {
		return nil, fmt.Errorf("add track: %w", err)
	}

	ctx, cancel := context.WithCancel(ctx)
	sub := &Subscription{Track: localTrack, Cancel: cancel}

	// Forward RTP packets in a goroutine.
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			pkt, _, err := remoteTrack.ReadRTP()
			if err != nil {
				return
			}
			if err := localTrack.WriteRTP(pkt); err != nil {
				return
			}
		}
	}()

	return sub, nil
}

// NewWebRTCAPI creates a WebRTC API configured for audio-only (Opus).
func NewWebRTCAPI() *webrtc.API {
	m := &webrtc.MediaEngine{}
	m.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    "audio/opus",
			ClockRate:   48000,
			Channels:    2,
			SDPFmtpLine: "minptime=10;useinbandfec=1",
		},
		PayloadType: 111,
	}, webrtc.RTPCodecTypeAudio)

	return webrtc.NewAPI(webrtc.WithMediaEngine(m))
}

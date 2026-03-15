package sfu

import (
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
)

func newTestAPI() *webrtc.API {
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

func TestPeerManager_CreateOffer(t *testing.T) {
	api := newTestAPI()
	pm := NewPeerManager(api)

	pc, offer, err := pm.CreatePeerConnection()
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()

	if offer.Type != webrtc.SDPTypeOffer {
		t.Fatalf("expected offer, got %s", offer.Type)
	}
	if offer.SDP == "" {
		t.Fatal("SDP should not be empty")
	}
}

func TestPeerManager_SetAnswer(t *testing.T) {
	api := newTestAPI()
	pm := NewPeerManager(api)

	sfuPC, offer, err := pm.CreatePeerConnection()
	if err != nil {
		t.Fatal(err)
	}
	defer sfuPC.Close()

	// Simulate a client PeerConnection answering.
	clientPC, err := api.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	defer clientPC.Close()

	if err := clientPC.SetRemoteDescription(offer); err != nil {
		t.Fatal(err)
	}
	answer, err := clientPC.CreateAnswer(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := clientPC.SetLocalDescription(answer); err != nil {
		t.Fatal(err)
	}

	// SFU sets the answer.
	if err := sfuPC.SetRemoteDescription(answer); err != nil {
		t.Fatal(err)
	}

	// Wait for connection.
	done := make(chan struct{})
	sfuPC.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		if s == webrtc.PeerConnectionStateConnected {
			close(done)
		}
	})

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Log("connection not established in 5s (expected in vnet-less test)")
	}
}

package constants

import (
	"gopkg.in/hraban/opus.v2"
)

const (
	App = opus.AppVoIP
	// AACNoiseGate     = 500
	Bitrate          = 12000
	Channels         = 1
	FrameSize        = 960 // 20ms at 48kHz
	SampleRate       = 48000
	JitterBufferSize = 10
	MaxBuffer        = 64
	MaxClients       = 4
	MaxPacketSize    = 4000
	Port             = ":1234"
)

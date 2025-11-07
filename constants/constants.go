package constants

import "gopkg.in/hraban/opus.v2"

const (
	App              = opus.AppVoIP
	AACNoiseGate     = 80
	Bitrate          = 12000
	Channels         = 1
	FrameSize        = 960 // 20ms at 48kHz
	JitterBufferSize = 4
	MaxBuffer        = 32
	MaxClients       = 4
	MaxPacketSize    = 4000
	Port             = ":1234"
	SampleRate       = 48000
)

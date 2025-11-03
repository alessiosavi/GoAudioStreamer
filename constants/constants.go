package constants

import "github.com/hraban/opus"

const (
	App              = opus.AppRestrictedLowdelay
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

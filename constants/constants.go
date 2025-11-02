package constants

import "github.com/hraban/opus"

const (
	Port          = ":1234"
	MaxClients    = 4
	SampleRate    = 48000
	Channels      = 1
	FrameSize     = 960 // 20ms at 48kHz
	App           = opus.AppRestrictedLowdelay
	Bitrate       = 12000
	MaxPacketSize = 4000
	MaxBuffer     = 32
)

package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"go-audio-streamer/constants"
	"go-audio-streamer/utils"
	"io"
	"net"

	"github.com/gordonklaus/portaudio"
	"github.com/hraban/opus"
	log "github.com/sirupsen/logrus"
)

func init() {
	// log.SetFlags(log.LstdFlags | log.LUTC | log.Llongfile | log.Lmicroseconds)
	utils.SetLog()
}

func main() {
	var host, port, password string

	flag.StringVar(&password, "password", "", "Password for authentication")
	flag.StringVar(&host, "host", "0.0.0.0", "Host to connect")
	flag.StringVar(&port, "port", "1234", "Port to connect")
	flag.Parse()

	if password == "" {
		log.Fatal("Password required; use -password=<yourpass>")
	}

	remoteAddress := fmt.Sprintf("%s:%s", host, port)
	log.Infof("Connecting to %s using password %s", remoteAddress, password)
	conn, err := net.Dial("tcp", remoteAddress)
	if err != nil {
		log.Fatal("Failed to connect:", err)
	}
	defer conn.Close()

	passBytes := []byte(password)
	lenBuf := make([]byte, constants.MaxBuffer)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(passBytes)))
	conn.Write(lenBuf)
	conn.Write(passBytes)

	// Receive ID
	idBuf := make([]byte, 1)
	if _, err = conn.Read(idBuf); err != nil {
		log.Fatal("Failed to receive ID:", err)
	}
	id := idBuf[0]
	log.Infof("Connected as client ID %d", id)

	if err := portaudio.Initialize(); err != nil {
		log.Fatal(err)
	}
	defer portaudio.Terminate()

	// Setup encoder
	enc, err := opus.NewEncoder(constants.SampleRate, constants.Channels, constants.App)
	if err != nil {
		log.Fatal("Failed to create encoder:", err)
	}
	enc.SetDTX(true)
	enc.SetBitrate(constants.Bitrate)

	// Setup decoder
	dec, err := opus.NewDecoder(constants.SampleRate, constants.Channels)
	if err != nil {
		log.Fatal("Failed to create decoder:", err)
	}

	// Input stream (microphone)
	inputBuffer := make([]int16, constants.FrameSize)
	inStream, err := portaudio.OpenDefaultStream(constants.Channels, 0, float64(constants.SampleRate), constants.FrameSize, inputBuffer)
	if err != nil {
		log.Fatal("Failed to open input stream:", err)
	}
	defer inStream.Close()

	// Output stream (speakers)
	outputBuffer := make([]int16, constants.FrameSize)
	outStream, err := portaudio.OpenDefaultStream(0, constants.Channels, float64(constants.SampleRate), constants.FrameSize, outputBuffer)
	if err != nil {
		log.Fatal("Failed to open output stream:", err)
	}
	defer outStream.Close()

	inStream.Start()
	outStream.Start()

	// Goroutine: Capture mic, encode, send
	go func() {
		for {
			if err := inStream.Read(); err != nil {
				log.Error("Input read error:", err)
				return
			}

			// Allocate buffer for encoded packet
			packet := make([]byte, constants.MaxPacketSize)
			n, err := enc.Encode(inputBuffer, packet)
			if err != nil {
				log.Error("Encode error:", err)
				continue
			}
			if n <= 20 {
				continue // Skip empty packets
			}
			log.Debugf("PACKET SIZE: %d", n)

			packet = packet[:n] // Slice to actual encoded size

			lenBuf := make([]byte, constants.MaxBuffer)
			binary.BigEndian.PutUint32(lenBuf, uint32(n))

			conn.Write(lenBuf)
			conn.Write(packet)
		}
	}()

	// Main loop: Receive mixed audio, decode, play
	for {
		lenBuf := make([]byte, constants.MaxBuffer)
		if _, err := io.ReadFull(conn, lenBuf); err != nil {
			log.Warn("Receive error:", err)
			return
		}
		packetLen := binary.BigEndian.Uint32(lenBuf)

		packet := make([]byte, packetLen)
		if _, err = io.ReadFull(conn, packet); err != nil {
			log.Warn("Receive error:", err)
			return
		}

		if _, err = dec.Decode(packet, outputBuffer); err != nil {
			log.Warn("Decode error:", err)
			continue
		}

		if err = outStream.Write(); err != nil {
			log.Error("Output write error:", err)
			return
		}
	}
}

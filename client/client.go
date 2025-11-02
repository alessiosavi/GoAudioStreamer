package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"os"

	"github.com/gordonklaus/portaudio"
	"github.com/hraban/opus"
)

const (
	sampleRate    = 48000
	channels      = 1
	frameSize     = 960 // 20ms at 48kHz
	app           = opus.AppVoIP
	bitrate       = 12000
	maxPacketSize = 4000
)

func main() {
	var host string
	if len(os.Args) != 2 {
		fmt.Println("Usage: go run client.go <server-ip:port>")
		host = "0.0.0.0:1234"
		// os.Exit(1)
	} else {
		host = os.Args[1]
	}

	conn, err := net.Dial("tcp", host)
	if err != nil {
		log.Fatal("Failed to connect:", err)
	}
	defer conn.Close()

	// Receive ID
	idBuf := make([]byte, 1)
	_, err = conn.Read(idBuf)
	if err != nil {
		log.Fatal("Failed to receive ID:", err)
	}
	id := idBuf[0]
	fmt.Printf("Connected as client ID %d\n", id)

	if err := portaudio.Initialize(); err != nil {
		panic(err)
	}
	defer portaudio.Terminate()

	// Setup encoder
	enc, err := opus.NewEncoder(sampleRate, channels, app)
	if err != nil {
		log.Fatal("Failed to create encoder:", err)
	}
	enc.SetBitrate(bitrate)

	// Setup decoder
	dec, err := opus.NewDecoder(sampleRate, channels)
	if err != nil {
		log.Fatal("Failed to create decoder:", err)
	}

	// Input stream (microphone)
	inputBuffer := make([]int16, frameSize)
	inStream, err := portaudio.OpenDefaultStream(channels, 0, float64(sampleRate), frameSize, inputBuffer)
	if err != nil {
		log.Fatal("Failed to open input stream:", err)
	}
	defer inStream.Close()
	inStream.Start()

	// Output stream (speakers)
	outputBuffer := make([]int16, frameSize)
	outStream, err := portaudio.OpenDefaultStream(0, channels, float64(sampleRate), frameSize, outputBuffer)
	if err != nil {
		log.Fatal("Failed to open output stream:", err)
	}
	defer outStream.Close()
	outStream.Start()

	// Goroutine: Capture mic, encode, send
	go func() {
		for {
			err := inStream.Read()
			if err != nil {
				log.Println("Input read error:", err)
				return
			}

			// Allocate buffer for encoded packet
			packet := make([]byte, maxPacketSize)
			n, err := enc.Encode(inputBuffer, packet)
			if err != nil {
				log.Println("Encode error:", err)
				continue
			}
			if n == 0 {
				continue // Skip empty packets
			}
			packet = packet[:n] // Slice to actual encoded size

			lenBuf := make([]byte, 4)
			binary.BigEndian.PutUint32(lenBuf, uint32(n))

			conn.Write(lenBuf)
			conn.Write(packet)
		}
	}()

	// Main loop: Receive mixed audio, decode, play
	for {
		lenBuf := make([]byte, 4)
		_, err := io.ReadFull(conn, lenBuf)
		if err != nil {
			log.Println("Receive error:", err)
			return
		}
		packetLen := binary.BigEndian.Uint32(lenBuf)

		packet := make([]byte, packetLen)
		_, err = io.ReadFull(conn, packet)
		if err != nil {
			log.Println("Receive error:", err)
			return
		}

		_, err = dec.Decode(packet, outputBuffer)
		if err != nil {
			log.Println("Decode error:", err)
			continue
		}

		err = outStream.Write()
		if err != nil {
			log.Println("Output write error:", err)
			return
		}
	}
}

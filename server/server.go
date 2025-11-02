package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"github.com/hraban/opus"
)

const (
	port          = ":1234"
	maxClients    = 4
	sampleRate    = 48000
	channels      = 1
	frameSize     = 960 // 20ms at 48kHz
	app           = opus.AppVoIP
	bitrate       = 12000
	maxPacketSize = 4000
)

var (
	clients  = make(map[int]net.Conn)
	pcmChans = make(map[int]chan []int16)
	mu       sync.Mutex
	nextID   = 0
)

func main() {
	ln, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Server listening on %s\n", port)

	// Start mixer goroutine
	go mixer()

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Println(err)
			continue
		}

		mu.Lock()
		if len(clients) >= maxClients {
			conn.Close()
			mu.Unlock()
			continue
		}
		nextID++
		id := nextID
		clients[id] = conn
		pcmChans[id] = make(chan []int16, 10) // Buffered to handle slight jitter
		mu.Unlock()

		// Send ID to client
		buf := []byte{byte(id)}
		conn.Write(buf)

		go handleClient(conn, id)
	}
}

func handleClient(conn net.Conn, id int) {
	defer func() {
		mu.Lock()
		delete(clients, id)
		delete(pcmChans, id)
		mu.Unlock()
		conn.Close()
	}()

	dec, err := opus.NewDecoder(sampleRate, channels)
	if err != nil {
		log.Printf("Failed to create decoder for client %d: %v", id, err)
		return
	}

	for {
		lenBuf := make([]byte, 4)
		_, err := io.ReadFull(conn, lenBuf)
		if err != nil {
			log.Printf("Client %d disconnected: %v", id, err)
			return
		}
		packetLen := binary.BigEndian.Uint32(lenBuf)

		packet := make([]byte, packetLen)
		_, err = io.ReadFull(conn, packet)
		if err != nil {
			log.Printf("Client %d disconnected: %v", id, err)
			return
		}

		pcm := make([]int16, frameSize)
		_, err = dec.Decode(packet, pcm)
		if err != nil {
			log.Printf("Decode error for client %d: %v", id, err)
			continue
		}

		pcmChans[id] <- pcm
	}
}

func mixer() {
	enc, err := opus.NewEncoder(sampleRate, channels, app)
	if err != nil {
		log.Fatal("Failed to create encoder:", err)
	}
	enc.SetBitrate(bitrate)

	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		mu.Lock()
		mixed := make([]int16, frameSize)
		for _, ch := range pcmChans {
			select {
			case pcm := <-ch:
				for i := 0; i < frameSize; i++ {
					mixed[i] += pcm[i]
					if mixed[i] > 32767 {
						mixed[i] = 32767
					} else if mixed[i] < -32768 {
						mixed[i] = -32768
					}
				}
			default:
				// No data, treat as silence
			}
		}
		mu.Unlock()

		if len(clients) < 2 {
			continue // No need to send if fewer than 2 clients
		}

		// Allocate buffer for encoded packet
		packet := make([]byte, maxPacketSize)
		n, err := enc.Encode(mixed, packet)
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

		mu.Lock()
		for _, conn := range clients {
			conn.Write(lenBuf)
			conn.Write(packet)
		}
		mu.Unlock()
	}
}

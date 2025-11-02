package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"go-audio-streamer/constants"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"github.com/hraban/opus"
)

var (
	clients  = make(map[int]net.Conn)
	pcmChans = make(map[int]chan []int16)
	mu       sync.Mutex
	nextID   = 0
	password = ""
)

func init() {
	log.SetFlags(log.LstdFlags | log.LUTC | log.Llongfile | log.Lmicroseconds)
}
func main() {

	flag.StringVar(&password, "password", "", "Password for authentication")
	flag.Parse()

	if password == "" {
		log.Fatal("Password required; use -password=<yourpass>")
	}

	ln, err := net.Listen("tcp", constants.Port)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Server listening on %s\n", constants.Port)

	// Start mixer goroutine
	go mixer()

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Println(err)
			continue
		}

		mu.Lock()
		if len(clients) >= constants.MaxClients {
			conn.Close()
			mu.Unlock()
			continue
		}
		nextID++
		id := nextID
		clients[id] = conn
		pcmChans[id] = make(chan []int16, 3) // Buffered to handle slight jitter
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

	// Read password length
	lenBuf := make([]byte, constants.MaxBuffer)
	if _, err := io.ReadFull(conn, lenBuf); err != nil {
		log.Printf("Client %d auth failed: %v", id, err)
		return
	}
	passLen := binary.BigEndian.Uint32(lenBuf)

	// Read password
	passBuf := make([]byte, passLen)
	if _, err := io.ReadFull(conn, passBuf); err != nil {
		log.Printf("Client %d auth failed: %v", id, err)
		return
	}
	receivedPassword := string(passBuf)
	log.Println(passLen, receivedPassword)

	// FIXME: use hashing like bcrypt
	if receivedPassword != password {
		log.Printf("Client %d invalid password", id)
		return
	}
	log.Printf("Client %d authenticated successfully", id)

	// Proceed with decoder setup and loop
	dec, err := opus.NewDecoder(constants.SampleRate, constants.Channels)
	if err != nil {
		log.Printf("Failed to create decoder for client %d: %v", id, err)
		return
	}

	for {
		lenBuf := make([]byte, constants.MaxBuffer)
		if _, err = io.ReadFull(conn, lenBuf); err != nil {
			log.Printf("Client %d disconnected: %v", id, err)
			return
		}
		packetLen := binary.BigEndian.Uint32(lenBuf)

		packet := make([]byte, packetLen)
		if _, err = io.ReadFull(conn, packet); err != nil {
			log.Printf("Client %d disconnected: %v", id, err)
			return
		}

		pcm := make([]int16, constants.FrameSize)
		if _, err = dec.Decode(packet, pcm); err != nil {
			log.Printf("Decode error for client %d: %v", id, err)
			continue
		}

		pcmChans[id] <- pcm
	}
}

func mixer() {
	enc, err := opus.NewEncoder(constants.SampleRate, constants.Channels, constants.App)
	if err != nil {
		log.Fatal("Failed to create encoder:", err)
	}
	enc.SetBitrate(constants.Bitrate)

	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		mu.Lock()
		mixed := make([]int16, constants.FrameSize)
		for _, ch := range pcmChans {
			select {
			case pcm := <-ch:
				for i := 0; i < constants.FrameSize; i++ {
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
		packet := make([]byte, constants.MaxPacketSize)
		n, err := enc.Encode(mixed, packet)
		if err != nil {
			log.Println("Encode error:", err)
			continue
		}
		if n == 0 {
			continue // Skip empty packets
		}
		packet = packet[:n] // Slice to actual encoded size

		lenBuf := make([]byte, constants.MaxBuffer)
		binary.BigEndian.PutUint32(lenBuf, uint32(n))

		mu.Lock()
		for _, conn := range clients {
			conn.Write(lenBuf)
			conn.Write(packet)
		}
		mu.Unlock()
	}
}

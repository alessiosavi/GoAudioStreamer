package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"go-audio-streamer/constants"
	"go-audio-streamer/utils"
	"io"
	"net"
	"os"
	"os/signal"
	"runtime/pprof"
	"sync"
	"syscall"
	"time"

	"github.com/alessiosavi/GoGPUtils/helper"
	log "github.com/sirupsen/logrus"
	"gopkg.in/hraban/opus.v2"
)

var (
	clients  = make(map[int]net.Conn)
	pcmChans = make(map[int]chan []int16)
	pcmPool  = sync.Pool{
		New: func() any {
			b := make([]int16, constants.MaxPacketSize)
			return &b
		},
	}
	mu       sync.Mutex
	nextID   = 0
	password = ""
)

func init() {
	// log.SetFlags(log.LstdFlags | log.LUTC | log.Llongfile | log.Lmicroseconds)
	utils.SetLog(log.DebugLevel)
	utils.SetLog(log.DebugLevel)
}
func main() {
	var generateProf bool
	flag.StringVar(&password, "password", "", "Password for authentication")
	flag.BoolVar(&generateProf, "pprof", false, "Generate optimization file")

	flag.Parse()

	if password == "" {
		log.Fatal("Password required; use -password=<yourpass>")
	}

	if generateProf {
		os.Mkdir("pprof", 0755)
		if f, err := os.Create(fmt.Sprintf("pprof/cpu-server-%s.pprof", helper.InitRandomizer().RandomString(6))); err == nil {
			defer f.Close()
			if err := pprof.StartCPUProfile(f); err != nil {
				log.Fatal(err)
			}
			defer pprof.StopCPUProfile()
		} else {
			log.Warning("Unable to create pprof file")
		}
	}
	// Handle signals for clean shutdown
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	ln, err := net.Listen("tcp", constants.Port)
	if err != nil {
		log.Fatal(err)
	}
	log.Infof("Server listening on %s", constants.Port)

	go func() {
		<-sigs
		pprof.StopCPUProfile()
		for _, client := range clients {
			client.Close()
		}
		ln.Close()
		os.Exit(0)
	}()
	// Start mixer goroutine
	go mixer()

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Warnf("Error during accepting client: %s", err.Error())
			continue
		}

		mu.Lock()
		if len(clients) >= constants.MaxClients {
			log.Warnf("We already have %d clients, dropping %s", len(clients), conn.RemoteAddr().String())
			conn.Close()
			mu.Unlock()
			continue
		}
		nextID++
		id := nextID
		clients[id] = conn
		pcmChans[id] = make(chan []int16, constants.JitterBufferSize) // Buffered to handle slight jitter
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
		log.Errorf("Client %d auth failed: %v", id, err)
		return
	}
	passLen := binary.BigEndian.Uint32(lenBuf)

	// Read password
	passBuf := make([]byte, passLen)
	if _, err := io.ReadFull(conn, passBuf); err != nil {
		// FIXME: Just for debug auth
		log.Debugf("Client %d auth failed: %v | Password: %s", id, err, string(passBuf))
		return
	}
	receivedPassword := string(passBuf)
	if receivedPassword != password {
		// FIXME: Just for debug auth
		log.Debugf("Client %d invalid password: <%s>", id, receivedPassword)
		return
	}
	log.Infof("Client %d authenticated successfully", id)

	// Setup decoder
	dec, err := opus.NewDecoder(constants.SampleRate, constants.Channels)
	if err != nil {
		log.Errorf("Failed to create decoder for client %d: %v", id, err)
		return
	}

	// Create pools
	var packetPool = sync.Pool{
		New: func() any {
			b := make([]byte, constants.MaxPacketSize)
			return &b
		},
	}

	clear(lenBuf)

	for {
		// Read packet length
		if _, err = io.ReadFull(conn, lenBuf); err != nil {
			log.Warnf("Client %d disconnected: %v", id, err)
			return
		}
		packetLen := binary.BigEndian.Uint32(lenBuf)

		// Validate packet length
		if packetLen > constants.MaxPacketSize {
			log.Errorf("Client %d sent oversized packet: %d", id, packetLen)
			return
		}

		// Get buffer from pool
		packetPtr := packetPool.Get().(*[]byte)
		packet := *packetPtr
		if uint32(cap(packet)) < packetLen {
			packet = make([]byte, packetLen)
		} else {
			packet = packet[:packetLen]
		}

		// Read audio packet
		if _, err = io.ReadFull(conn, packet); err != nil {
			log.Warnf("Client %d disconnected: %v", id, err)
			packetPool.Put(&packet)
			return
		}

		log.Tracef("Read from client <%d> packet: %+v", id, packet)

		// Get PCM buffer from pool and decode
		pcmPtr := pcmPool.Get().(*[]int16)
		pcm := *pcmPtr
		if _, err = dec.Decode(packet, pcm); err != nil {
			log.Errorf("Decode error for client %d: %v", id, err)
			pcmPool.Put(&pcm)
			packetPool.Put(&packet)
			continue
		}
		log.Tracef("Decoded from client <%d> PCM: %+v", id, pcm)

		// Send to mixer (non-blocking)
		select {
		case pcmChans[id] <- pcm:
			log.Tracef("Client <%d> sent PCM to the mixer", id)
			// Success - pcm ownership transferred to mixer
		default:
			// Channel full - drop audio and return buffer to pool
			pcmPool.Put(&pcm)
			log.Warnf("Client %d audio dropped - mixer overloaded", id)
		}

		packetPool.Put(&packet)
	}
}

func mixer() {
	enc, err := opus.NewEncoder(constants.SampleRate, constants.Channels, constants.App)
	if err != nil {
		log.Fatal("Failed to create encoder:", err)
	}
	enc.SetDTX(true)
	enc.SetBitrate(constants.Bitrate)

	ticker := time.NewTicker((constants.FrameSize / 48) * time.Millisecond)
	defer ticker.Stop()

	packet := make([]byte, constants.MaxPacketSize)
	lenBuf := make([]byte, constants.MaxBuffer)
	for range ticker.C {
		// Clear mixed buffer for new mix
		mixed := make([]int16, constants.FrameSize)

		activeClients := 0

		mu.Lock()
		// Collect active clients and mix audio
		for _, ch := range pcmChans {
			select {
			case pcm := <-ch:
				// clientIDs = append(clientIDs, id)
				activeClients++

				// Mix audio with clipping protection
				for i := range mixed {
					// Convert to int32 to prevent overflow during sum
					sum := int32(mixed[i]) + int32(pcm[i])

					// Clamp to int16 range
					if sum > 32767 {
						sum = 32767
					} else if sum < -32768 {
						sum = -32768
					}
					mixed[i] = int16(sum)
				}

				// Return PCM buffer to the GLOBAL pool
				pcmPool.Put(&mixed) // Use the global pool, not a local one

			default:
				// No data from this client
			}
		}
		clientCount := len(clients)
		mu.Unlock()

		// Skip if no active audio or only one client
		if activeClients == 0 || clientCount < 2 {
			continue
		}

		// Encode mixed audio
		n, err := enc.Encode(mixed, packet)
		if err != nil {
			log.Error("Encode error:", err)
			continue
		}
		if n == 0 {
			continue // Skip empty packets (DTX)
		}

		// Prepare length header
		binary.BigEndian.PutUint32(lenBuf, uint32(n))
		validPacket := packet[:n]

		// Broadcast to all clients
		mu.Lock()
		for _, clientConn := range clients {
			// Write length header
			if _, err := clientConn.Write(lenBuf); err != nil {
				log.Warnf("Write error to client: %v", err)
				continue
			}

			// Write audio packet
			if _, err := clientConn.Write(validPacket); err != nil {
				log.Warnf("Write error to client: %v", err)
			}
		}
		mu.Unlock()
	}
}

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
	"strings"
	"syscall"
	"time"

	"github.com/alessiosavi/GoGPUtils/helper"
	"github.com/gordonklaus/portaudio"
	log "github.com/sirupsen/logrus"
	"gopkg.in/hraban/opus.v2"
)

func init() {
	utils.SetLog(log.DebugLevel)
}

func main() {
	var host, port, password string
	var generateProf bool
	// var denoiser bool

	flag.StringVar(&password, "password", "test", "Password for authentication")
	flag.StringVar(&host, "host", "0.0.0.0", "Host to connect")
	flag.StringVar(&port, "port", "1234", "Port to connect")
	flag.BoolVar(&generateProf, "pprof", false, "Generate optimization file")
	// flag.BoolVar(&denoiser, "denoiser", false, "Reduce noise using AAC")

	flag.Parse()

	if password == "" {
		log.Fatal("Password required; use -password=<yourpass>")
	}

	if generateProf {
		os.Mkdir("pprof", 0755)
		f, err := os.Create(fmt.Sprintf("pprof/cpu-client-%s.pprof", helper.InitRandomizer().RandomString(6)))
		if err == nil {
			defer f.Close()
			if err := pprof.StartCPUProfile(f); err != nil {
				log.Warningf("Unable to generate pprof: %s", err)
			}

		} else {
			log.Warningf("Unable to generate pprof: %s", err)
		}

	}

	// Handle signals for clean shutdown
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	remoteAddress := net.JoinHostPort(host, port)
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

	// Jitter buffer for decoded PCM frames
	jitterBufferSize := 4 // ~80ms buffer; adjust for network
	outputQueue := make(chan []int16, jitterBufferSize)

	// Silence buffer for underruns
	silenceBuffer := make([]int16, constants.FrameSize)
	outputBuffer := make([]int16, constants.FrameSize)

	// Input stream setup with restart logic
	var inStream *portaudio.Stream
	inputBuffer := make([]int16, constants.FrameSize)

	// Output stream setup with restart logic
	var outStream *portaudio.Stream
	// Capture goroutine: Mic read, encode, send with auto-restart
	go func() {
		packet := make([]byte, constants.MaxPacketSize)
		lenBuf := make([]byte, constants.MaxBuffer)

		for { // Outer restart loop
			inStream = initInputStream(&inputBuffer)
			defer inStream.Close()
			log.Info("Input stream (re)started")

			for {
				if err := inStream.Read(); err != nil {
					if strings.Contains(err.Error(), "Invalid stream pointer") || strings.Contains(err.Error(), "Stream is stopped") || strings.Contains(err.Error(), "Input/output error") {
						log.Warnf("Recoverable input error: %v; restarting stream", err)
						break // To outer loop (restarts stream)
					}
					log.Errorf("Fatal input read error: %v", err)
					return
				}
				n, err := enc.Encode(inputBuffer, packet)
				if err != nil {
					log.Error("Encode error:", err)
					continue
				}

				validPacket := packet[:n]
				binary.BigEndian.PutUint32(lenBuf, uint32(n))

				conn.Write(lenBuf)
				conn.Write(validPacket)

				packet = packet[:constants.MaxPacketSize]
			}
		}
	}()
	// Playback goroutine: Ticks to play from queue or silence
	go func() {
		ticker := time.NewTicker((constants.FrameSize / 48) * time.Millisecond)
		defer ticker.Stop()

		for range ticker.C {
			if outStream == nil {
				continue // Wait for stream init in main loop
			}

			var pcm []int16
			select {
			case pcm = <-outputQueue:
			default:
				pcm = silenceBuffer // Play silence to prevent underrun
			}

			copy(outputBuffer, pcm)

			if err := outStream.Write(); err != nil { // Assumes buffer is pcm (binding uses last Open arg as buffer)
				if strings.Contains(err.Error(), "Output underflowed") {
					log.Warn("Ignoring output underflow")
					continue
				}
				log.Warnf("Playback error: %v; restarting output stream", err)
				outStream.Close()
				outStream = nil // Trigger restart
			}
		}
	}()

	go func() {
		<-sigs
		pprof.StopCPUProfile()
		outStream.Close()
		inStream.Close()
		conn.Close()
		os.Exit(0)

	}()

	// Main loop: Receive, decode, queue; with output restart
	for { // Outer loop for output restart
		if outStream == nil {
			// Use silence initially;
			outStream = initOutputStream(&outputBuffer)
		}

		lenBuf := make([]byte, constants.MaxBuffer)
		if _, err := io.ReadFull(conn, lenBuf); err != nil {
			log.Warn("Receive error:", err)
			return // Fatal network
		}
		packetLen := binary.BigEndian.Uint32(lenBuf)

		if packetLen == 0 {
			continue // Ignore empty
		}

		log.Tracef("Packet size: %d", packetLen)

		packet := make([]byte, packetLen)
		if _, err = io.ReadFull(conn, packet); err != nil {
			log.Warn("Receive error:", err)
			return
		}

		outputBuffer := make([]int16, constants.FrameSize)
		if _, err = dec.Decode(packet, outputBuffer); err != nil {
			log.Warn("Decode error:", err)
			continue
		}

		// Queue (drop if full to avoid backlog/latency)
		select {
		case outputQueue <- outputBuffer:
		default:
			log.Debug("Jitter buffer full; dropping frame")
		}
	}

}

func initOutputStream(buffer *[]int16) *portaudio.Stream {
	outStream, err := portaudio.OpenDefaultStream(0, constants.Channels, float64(constants.SampleRate), constants.FrameSize, buffer)
	if err != nil {
		log.Fatalf("Open output error: %v; retrying in 1s", err)
	}
	if err = outStream.Start(); err != nil {
		outStream.Close()
		log.Fatalf("Start output error: %v; retrying in 1s", err)
	}
	log.Info("Output stream (re)started")
	return outStream
}

func initInputStream(buffer *[]int16) *portaudio.Stream {
	inStream, err := portaudio.OpenDefaultStream(constants.Channels, 0, float64(constants.SampleRate), constants.FrameSize, buffer)
	if err != nil {
		log.Fatalf("Open input error: %v; retrying in 500 ms", err)
	}

	if err = inStream.Start(); err != nil {
		inStream.Close()
		log.Fatalf("Start input error: %v; retrying in 500 ms", err)
	}
	log.Info("Input stream (re)started")
	return inStream
}

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"go-audio-streamer/constants"
	pb "go-audio-streamer/proto"
	"go-audio-streamer/utils"

	"github.com/alessiosavi/GoGPUtils/helper"
	mathutils "github.com/alessiosavi/GoGPUtils/math"
	"github.com/gordonklaus/portaudio"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gopkg.in/hraban/opus.v2"

	"sync"
)

// AudioClient represents the audio streaming client with input and output streams.
type AudioClient struct {
	conn      *grpc.ClientConn
	client    pb.AudioServiceClient
	enc       *opus.Encoder
	dec       *opus.Decoder
	inStream  *portaudio.Stream
	outStream *portaudio.Stream
	inBuffer  []int16
	outBuffer []int16
	packet    []byte
	mu        sync.RWMutex // Protects stream fields for concurrent access
}

// NewAudioClient creates a new AudioClient instance.
func NewAudioClient(ctx context.Context) (*AudioClient, error) {
	conn, err := grpc.NewClient("0.0.0.0:50051",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}

	client := pb.NewAudioServiceClient(conn)

	if err := portaudio.Initialize(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to initialize PortAudio: %w", err)
	}

	enc, err := opus.NewEncoder(constants.SampleRate, constants.Channels, constants.App)
	if err != nil {
		conn.Close()
		portaudio.Terminate()
		return nil, fmt.Errorf("failed to create encoder: %w", err)
	}
	enc.SetDTX(true)
	enc.SetBitrate(constants.Bitrate)
	dec, err := opus.NewDecoder(constants.SampleRate, constants.Channels)
	if err != nil {
		conn.Close()
		portaudio.Terminate()
		return nil, fmt.Errorf("failed to create decoder: %w", err)
	}

	return &AudioClient{
		conn:      conn,
		client:    client,
		enc:       enc,
		dec:       dec,
		inBuffer:  make([]int16, constants.FrameSize),
		outBuffer: make([]int16, constants.FrameSize),
		packet:    make([]byte, constants.FrameSize),
	}, nil
}

// ensure Stream is closed and nilled atomically.
func (ac *AudioClient) closeStream(stream **portaudio.Stream) {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	if *stream != nil {
		(*stream).Close()
		*stream = nil
	}
}

// getOrInitStream atomically gets or initializes a stream.
func (ac *AudioClient) getOrInitInStream() (*portaudio.Stream, error) {
	ac.mu.RLock()
	if ac.inStream != nil {
		ac.mu.RUnlock()
		return ac.inStream, nil
	}
	ac.mu.RUnlock()

	ac.mu.Lock()
	defer ac.mu.Unlock()
	if ac.inStream != nil {
		return ac.inStream, nil
	}

	stream := utils.InitInputStream(&ac.inBuffer)
	ac.inStream = stream
	return stream, nil
}

// getOrInitOutStream atomically gets or initializes the output stream.
func (ac *AudioClient) getOrInitOutStream() (*portaudio.Stream, error) {
	ac.mu.RLock()
	if ac.outStream != nil {
		ac.mu.RUnlock()
		return ac.outStream, nil
	}
	ac.mu.RUnlock()

	ac.mu.Lock()
	defer ac.mu.Unlock()
	if ac.outStream != nil {
		return ac.outStream, nil
	}

	stream := utils.InitOutputStream(&ac.outBuffer)
	ac.outStream = stream
	return stream, nil
}

// startInputStream starts the input streaming goroutine, which handles mic reading, encoding, and sending.
func (ac *AudioClient) startInputStream(ctx context.Context, stream pb.AudioService_StreamAudioClient) error {
	go func() error {
		defer func() {
			ac.closeStream(&ac.inStream)
		}()

		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			inStream, err := ac.getOrInitInStream()
			if err != nil {
				log.Errorf("Failed to initialize input stream: %v", err)
				time.Sleep(100 * time.Millisecond) // Backoff before retry
				continue
			}

			for {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
				}

				if err := inStream.Read(); err != nil {
					log.Errorf("Error reading from input stream: %v", err)
					ac.closeStream(&ac.inStream)
					break // Restart stream
				}

				n, err := ac.enc.Encode(ac.inBuffer, ac.packet)
				if err != nil {
					log.Errorf("Error encoding audio: %v", err)
					continue
				}
				if n <= 1 {
					log.Debug("Skipping empty packet")
					continue
				}

				validPacket := ac.packet[:n]
				chunk := &pb.AudioChunk{Data: validPacket, Timestamp: time.Now().UnixMicro()}
				if err := stream.Send(chunk); err != nil {
					log.Errorf("Send error: %v", err)
					return err
				}

				log.Debugf("Sent packet: AVG:[%f], LEN:%d", mathutils.Average(ac.inBuffer), n)
				// Reset buffer for next read
				for i := range ac.inBuffer {
					ac.inBuffer[i] = 0
				}
			}
		}
	}()
	return nil
}

// runOutputStream runs the output loop: receiving, decoding, and playing audio.
func (ac *AudioClient) runOutputStream(ctx context.Context, stream pb.AudioService_StreamAudioClient) error {
	defer func() {
		ac.closeStream(&ac.outStream)
	}()

	avg := utils.NewSlidingWindowAvg[int64](15)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		outStream, err := ac.getOrInitOutStream()
		if err != nil {
			log.Errorf("Failed to initialize output stream: %v", err)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			now := time.Now()
			chunk, err := stream.Recv()
			if err != nil {
				log.Errorf("Recv error: %v", err)
				return err
			}

			buffer := make([]int16, constants.FrameSize)
			n, err := ac.dec.Decode(chunk.Data, buffer)
			if err != nil {
				log.Errorf("Decode 	error:%v", err)
				continue
			}

			copy(ac.outBuffer, buffer[:n])

			if err := outStream.Write(); err != nil {
				log.Errorf("Error writing to output stream: %v", err)
				ac.closeStream(&ac.outStream)
				break // Restart stream
			}
			pcktTime := time.UnixMicro(chunk.Timestamp)
			processTime := now.Sub(pcktTime)
			avg.Push(processTime.Milliseconds())
			log.Debugf("[AVGProcess: %f:0.4,ProcessTime: %s | I/OTime: %s]Played chunk from %s: AVG:[%f], LEN:%d", avg.Avg(), processTime, time.Since(pcktTime), chunk.ClientId, mathutils.Average(buffer[:n]), n)
		}
	}
}

// Run starts the bidirectional audio stream.
func (ac *AudioClient) Run(ctx context.Context) error {
	stream, err := ac.client.StreamAudio(ctx)
	if err != nil {
		return fmt.Errorf("failed to create stream: %w", err)
	}
	defer stream.CloseSend()

	if err := ac.startInputStream(ctx, stream); err != nil {
		return fmt.Errorf("failed to start input stream: %w", err)
	}

	if err := ac.runOutputStream(ctx, stream); err != nil {
		return fmt.Errorf("output stream failed: %w", err)
	}

	return nil
}

// Shutdown cleanly shuts down the client.
func (ac *AudioClient) Shutdown() {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	if ac.inStream != nil {
		ac.inStream.Close()
		ac.inStream = nil
	}
	if ac.outStream != nil {
		ac.outStream.Close()
		ac.outStream = nil
	}
	if ac.conn != nil {
		ac.conn.Close()
	}
	portaudio.Terminate()
}

func init() {
	utils.SetLog(log.DebugLevel)
}

func main() {
	stopCPU := utils.StartCPUProfiling(filepath.Join("/tmp", fmt.Sprintf("cpu_client-%s.prof", helper.InitRandomizer().RandomString(5))))
	defer stopCPU()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client, err := NewAudioClient(ctx)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer client.Shutdown()

	// Handle shutdown signals
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigs
		log.Info("Received shutdown signal")
		cancel()
		client.Shutdown()
		stopCPU()
		os.Exit(0)
	}()

	if err := client.Run(ctx); err != nil {
		log.Errorf("Client run failed: %v", err)
	}

}

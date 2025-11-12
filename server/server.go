package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	pb "go-audio-streamer/proto"
	"go-audio-streamer/utils"

	"github.com/alessiosavi/GoGPUtils/helper"
	mathutils "github.com/alessiosavi/GoGPUtils/math"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

const (
	bufferSize = 1000 // per client
)

func init() {
	// log.SetFlags(log.LstdFlags | log.LUTC | log.Llongfile | log.Lmicroseconds)
	utils.SetLog(log.DebugLevel)
}

type client struct {
	id     string
	stream pb.AudioService_StreamAudioServer
	sendCh chan *pb.AudioChunk
	done   chan struct{}
}

type server struct {
	pb.UnimplementedAudioServiceServer
	mu          sync.RWMutex
	clients     map[string]*client
	broadcastCh chan *pb.AudioChunk
	wg          sync.WaitGroup
}

func NewServer() *server {
	s := &server{
		clients:     make(map[string]*client),
		broadcastCh: make(chan *pb.AudioChunk, 1000),
	}
	s.startBroadcaster()
	return s
}

func (s *server) startBroadcaster() {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for chunk := range s.broadcastCh {
			// s.mu.RLock()
			for id, c := range s.clients {
				if id == chunk.ClientId {
					continue // don't echo back to sender
				}
				select {
				case c.sendCh <- chunk:
					pcktTime := time.UnixMicro(chunk.Timestamp)
					log.Printf("[Time: %s]Sending data [%d] from <%s> to <%s> [<AVG:%f>]: <%s>\n", time.Since(pcktTime), len(chunk.Data), chunk.ClientId, id, mathutils.Average(chunk.Data), helper.Marshal(chunk))
				default:
					// Drop if client is slow (backpressure)
					log.Printf("Client %s buffer full, dropping packet", id)
				}
			}
			// s.mu.RUnlock()
		}
	}()
}

func (s *server) StreamAudio(stream pb.AudioService_StreamAudioServer) error {
	// Generate client ID (in real app: use UUID or auth)
	clientID := generateID()

	// Create client with buffered channel
	cl := &client{
		id:     clientID,
		stream: stream,
		sendCh: make(chan *pb.AudioChunk, bufferSize),
		done:   make(chan struct{}),
	}

	// Register client
	s.mu.Lock()
	s.clients[clientID] = cl
	s.mu.Unlock()
	defer s.cleanup(clientID)

	log.Printf("Client connected: %s", clientID)

	// Start sender goroutine (non-blocking send to client)
	go cl.sender()

	// Receive loop: non-blocking receive from client
	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			log.Printf("Client %s disconnected gracefully", clientID)
			return nil
		}
		if err != nil {
			log.Printf("Client %s error: %v", clientID, err)
			return err
		}

		// Attach client ID and broadcast
		chunk.ClientId = clientID
		select {
		case s.broadcastCh <- chunk:
			// Broadcasted
		default:
			log.Printf("Broadcast channel full, dropping packet from %s", clientID)
		}
	}
}

func (c *client) sender() {
	defer close(c.done)
	for chunk := range c.sendCh {
		if err := c.stream.Send(chunk); err != nil {
			return
		}
	}
}

func (s *server) cleanup(id string) {
	s.mu.Lock()
	if cl, ok := s.clients[id]; ok {
		close(cl.sendCh)
		<-cl.done
		delete(s.clients, id)
	}
	s.mu.Unlock()
}

func (s *server) Shutdown() {
	close(s.broadcastCh)
	s.wg.Wait()

	s.mu.Lock()
	for _, cl := range s.clients {
		close(cl.sendCh)
	}
	s.mu.Unlock()
}

var idCounter int
var idMu sync.Mutex

func generateID() string {
	idMu.Lock()
	defer idMu.Unlock()
	idCounter++
	return fmt.Sprintf("client-%d", idCounter)
}

func main() {
	stopCPU := utils.StartCPUProfiling(filepath.Join("/tmp", fmt.Sprintf("cpu_server-%s.prof", helper.InitRandomizer().RandomString(5))))
	defer stopCPU()
	lis, err := net.Listen("tcp", "0.0.0.0:50051")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	grpcSrv := grpc.NewServer()
	defer grpcSrv.GracefulStop()

	s := NewServer()
	defer s.Shutdown()
	pb.RegisterAudioServiceServer(grpcSrv, s)

	go func() {
		http.Handle("/metrics", promhttp.Handler())
		log.Fatal(http.ListenAndServe(":9091", nil))
	}()

	// Handle shutdown signals
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigs
		log.Info("Received shutdown signal")
		grpcSrv.Stop()
		s.Shutdown()
		for id := range s.clients {
			s.cleanup(id)
		}
		stopCPU()
		os.Exit(0)
	}()

	log.Println("Audio broadcast server running on :50051")
	if err := grpcSrv.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}

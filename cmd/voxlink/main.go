package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"

	"github.com/gordonklaus/portaudio"
	"go.uber.org/zap"

	"voxlink/internal/sfu"
	"voxlink/internal/signaling"
	"voxlink/internal/web"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	flag.Parse()

	logger, _ := zap.NewDevelopment()
	defer logger.Sync()
	sugar := logger.Sugar()

	if err := portaudio.Initialize(); err != nil {
		sugar.Fatalw("portaudio init failed", "err", err)
	}
	defer portaudio.Terminate()

	sfuEngine := sfu.New()
	defer sfuEngine.Close()

	webrtcAPI := sfu.NewWebRTCAPI()
	peerMgr := sfu.NewPeerManager(webrtcAPI)

	sigHandler := signaling.NewHandler(sfuEngine, logger, signaling.WithPeerManager(peerMgr))
	webHandler := web.NewHandler(sfuEngine, nil)

	mux := http.NewServeMux()
	mux.Handle("/ws", sigHandler)
	mux.Handle("/", webHandler)

	server := &http.Server{Addr: *addr, Handler: mux}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	go func() {
		sugar.Infow("VoxLink starting", "addr", *addr)
		fmt.Fprintf(os.Stderr, "\n  Open http://localhost%s in your browser\n\n", *addr)
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			sugar.Fatalw("server error", "err", err)
		}
	}()

	<-ctx.Done()
	sugar.Info("shutting down...")
	server.Shutdown(context.Background())
}

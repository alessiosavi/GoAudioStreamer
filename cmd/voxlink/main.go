package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"

	"github.com/gordonklaus/portaudio"

	"voxlink/internal/sfu"
	"voxlink/internal/signaling"
	"voxlink/internal/web"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	slog.SetDefault(logger)

	if err := portaudio.Initialize(); err != nil {
		logger.Error("portaudio init failed", "err", err)
		os.Exit(1)
	}
	defer portaudio.Terminate()

	sfuEngine := sfu.New()
	defer sfuEngine.Close()

	sigHandler := signaling.NewHandler(sfuEngine)
	webHandler := web.NewHandler(sfuEngine, nil)

	mux := http.NewServeMux()
	mux.Handle("/ws", sigHandler)
	mux.Handle("/", webHandler)

	server := &http.Server{Addr: *addr, Handler: mux}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	go func() {
		logger.Info("VoxLink starting", "addr", *addr)
		fmt.Fprintf(os.Stderr, "\n  Open http://localhost%s in your browser\n\n", *addr)
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			logger.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down...")
	server.Shutdown(context.Background())
}

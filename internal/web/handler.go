package web

import (
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"
	"strings"

	"voxlink/internal/sfu"
)

//go:embed static
var staticFiles embed.FS

// AudioDevice describes an audio input or output device.
type AudioDevice struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// AudioController is an interface for the web UI to control the native audio pipeline.
// It may be nil when no local audio pipeline is active (graceful no-op).
type AudioController interface {
	SetMute(muted bool) error
	SetVolume(peerID string, volume float64) error
	SetDenoise(enabled bool) error
	ListDevices() (inputs, outputs []AudioDevice, err error)
	SelectDevice(inputID, outputID string) error
}

// NewHandler creates the HTTP handler that serves the web UI and API endpoints.
// audioCtrl may be nil; all audio endpoints become graceful no-ops in that case.
func NewHandler(s *sfu.SFU, audioCtrl AudioController) http.Handler {
	mux := http.NewServeMux()

	staticFS, _ := fs.Sub(staticFiles, "static")
	fileServer := http.FileServer(http.FS(staticFS))

	// Root: serve index.html directly.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			data, _ := staticFiles.ReadFile("static/index.html")
			w.Write(data) //nolint:errcheck
			return
		}
		http.StripPrefix("/static/", fileServer).ServeHTTP(w, r)
	})

	// /static/* — serve embedded assets.
	mux.HandleFunc("/static/", func(w http.ResponseWriter, r *http.Request) {
		http.StripPrefix("/static/", fileServer).ServeHTTP(w, r)
	})

	// GET /api/health
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"}) //nolint:errcheck
	})

	// GET /api/room/:code
	mux.HandleFunc("/api/room/", func(w http.ResponseWriter, r *http.Request) {
		code := strings.TrimPrefix(r.URL.Path, "/api/room/")
		w.Header().Set("Content-Type", "application/json")
		room, ok := s.GetRoom(code)
		if !ok {
			json.NewEncoder(w).Encode(map[string]any{"exists": false, "peers": 0}) //nolint:errcheck
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"exists": true, "peers": room.PeerCount()}) //nolint:errcheck
	})

	// POST /api/audio/mute
	mux.HandleFunc("/api/audio/mute", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if audioCtrl != nil {
			var req struct {
				Muted bool `json:"muted"`
			}
			json.NewDecoder(r.Body).Decode(&req) //nolint:errcheck
			audioCtrl.SetMute(req.Muted)         //nolint:errcheck
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": true}) //nolint:errcheck
	})

	// POST /api/audio/volume
	mux.HandleFunc("/api/audio/volume", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if audioCtrl != nil {
			var req struct {
				PeerID string  `json:"peerId"`
				Volume float64 `json:"volume"`
			}
			json.NewDecoder(r.Body).Decode(&req)          //nolint:errcheck
			audioCtrl.SetVolume(req.PeerID, req.Volume)   //nolint:errcheck
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": true}) //nolint:errcheck
	})

	// POST /api/audio/denoise
	mux.HandleFunc("/api/audio/denoise", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if audioCtrl != nil {
			var req struct {
				Enabled bool `json:"enabled"`
			}
			json.NewDecoder(r.Body).Decode(&req)   //nolint:errcheck
			audioCtrl.SetDenoise(req.Enabled)      //nolint:errcheck
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": true}) //nolint:errcheck
	})

	// GET /api/audio/devices
	mux.HandleFunc("/api/audio/devices", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if audioCtrl == nil {
			json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
				"inputs":  []AudioDevice{},
				"outputs": []AudioDevice{},
			})
			return
		}
		inputs, outputs, _ := audioCtrl.ListDevices()
		if inputs == nil {
			inputs = []AudioDevice{}
		}
		if outputs == nil {
			outputs = []AudioDevice{}
		}
		json.NewEncoder(w).Encode(map[string]any{"inputs": inputs, "outputs": outputs}) //nolint:errcheck
	})

	// POST /api/audio/device
	mux.HandleFunc("/api/audio/device", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if audioCtrl != nil {
			var req struct {
				Input  string `json:"input"`
				Output string `json:"output"`
			}
			json.NewDecoder(r.Body).Decode(&req)              //nolint:errcheck
			audioCtrl.SelectDevice(req.Input, req.Output)     //nolint:errcheck
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": true}) //nolint:errcheck
	})

	return mux
}

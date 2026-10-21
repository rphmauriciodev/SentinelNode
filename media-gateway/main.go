package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/rphmauriciodev/SentinelNode/media-gateway/config"
	"github.com/rphmauriciodev/SentinelNode/media-gateway/internal/recorder"
	"github.com/rphmauriciodev/SentinelNode/media-gateway/internal/stream"
	"github.com/rphmauriciodev/SentinelNode/media-gateway/internal/web"
)

type RecordRequest struct {
	DeviceID        string `json:"device_id"`
	VideoURL        string `json:"video_url"`
	DurationSeconds int    `json:"duration_seconds"`
}

type StreamRegisterRequest struct {
	DeviceID  string `json:"device_id"`
	StreamURL string `json:"stream_url"`
}

var (
	recordingsDir       string
	serverPort          string
	streamManager       *stream.StreamManager
	recordingController *recorder.RecordingController
	metadataStore       *recorder.MetadataStore
)

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Erro ao carregar configuração: %v", err)
	}

	recordingsDir = cfg.RecordingsDir
	serverPort = cfg.Port

	metadataStore = recorder.NewMetadataStore(recordingsDir)
	tolerance := time.Duration(cfg.ToleranceSeconds) * time.Second
	if tolerance <= 0 {
		tolerance = 30 * time.Second
	}
	recordingController = recorder.NewRecordingController(recordingsDir, tolerance, metadataStore)
	streamManager = stream.NewStreamManager()

	if cfg.DefaultCameraURL != "" {
		devID := cfg.DefaultDeviceID
		if devID == "" {
			devID = "esp32-cam-01"
		}
		log.Printf("[Stream] Registrando câmera padrão: %s -> %s\n", devID, cfg.DefaultCameraURL)
		streamManager.Register(devID, cfg.DefaultCameraURL)
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" || r.URL.Path == "/dashboard" {
			web.Handler()(w, r)
			return
		}

		if strings.HasPrefix(r.URL.Path, "/recordings/") {
			handleServeRecording(w, r)
			return
		}
		http.NotFound(w, r)
	})

	mux.HandleFunc("/live", handleLive)
	mux.HandleFunc("/live/", handleLive)
	mux.HandleFunc("/stream", handleLive)
	mux.HandleFunc("/stream/", handleLive)
	mux.HandleFunc("/snapshot", handleSnapshot)
	mux.HandleFunc("/snapshot/", handleSnapshot)
	mux.HandleFunc("/stream/register", handleStreamRegister)
	mux.HandleFunc("/status", handleStatus)

	mux.HandleFunc("/record", handleRecord)
	mux.HandleFunc("/api/recordings", handleListRecordings)

	server := &http.Server{
		Addr:    fmt.Sprintf(":%s", cfg.Port),
		Handler: mux,
	}

	go func() {
		log.Printf("==================================================================")
		log.Printf("SENTINELNODE MEDIA GATEWAY ATIVO na porta :%s", cfg.Port)
		log.Printf("Interface Web de Vigilância: http://localhost:%s/", cfg.Port)
		log.Printf("Stream ao Vivo Centralizado: http://localhost:%s/live?device_id=esp32-cam-01", cfg.Port)
		log.Printf("Tolerância Inatividade PIR: %v", tolerance)
		log.Printf("==================================================================")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Erro ao iniciar servidor: %v", err)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan

	log.Println("\nEncerrando Media Gateway e finalizando gravações graciosamente...")

	recordingController.Shutdown()

	streamManager.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Erro durante shutdown do servidor HTTP: %v\n", err)
	}

	log.Println("SentinelNode Media Gateway finalizado com sucesso.")
}

func handleLive(w http.ResponseWriter, r *http.Request) {
	deviceID := extractDeviceID(r, "/live/", "/stream/")
	if deviceID == "" {
		http.Error(w, "device_id não informado (use query ?device_id=... ou rota /live/<id>)", http.StatusBadRequest)
		return
	}

	cs, found := streamManager.Get(deviceID)
	if !found {
		sourceURL := r.URL.Query().Get("source_url")
		if sourceURL != "" {
			cs = streamManager.Register(deviceID, sourceURL)
		} else {
			cs = streamManager.Register(deviceID, "")
		}
	}

	cs.ServeLiveHTTP(w, r)
}

func handleSnapshot(w http.ResponseWriter, r *http.Request) {
	deviceID := extractDeviceID(r, "/snapshot/")
	if deviceID == "" {
		http.Error(w, "device_id não informado", http.StatusBadRequest)
		return
	}

	cs, found := streamManager.Get(deviceID)
	if !found {
		http.Error(w, "câmera não encontrada", http.StatusNotFound)
		return
	}

	frame := cs.Broadcaster().LastFrame()
	if frame == nil {
		frame = stream.GeneratePlaceholderFrame(fmt.Sprintf("Aguardando câmera %s...", deviceID))
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(frame)))
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.WriteHeader(http.StatusOK)
	w.Write(frame)
}

func handleStreamRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
		return
	}

	var req StreamRegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Erro ao decodificar JSON", http.StatusBadRequest)
		return
	}

	if req.DeviceID == "" || req.StreamURL == "" {
		http.Error(w, "device_id e stream_url são obrigatórios", http.StatusBadRequest)
		return
	}

	cs := streamManager.Register(req.DeviceID, req.StreamURL)
	log.Printf("[Stream Manager] Câmera registrada: [%s] -> %s\n", req.DeviceID, req.StreamURL)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cs.Status())
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	streams := streamManager.GetAll()
	res := make([]map[string]interface{}, 0, len(streams))
	for _, cs := range streams {
		status := cs.Status()
		status["is_recording"] = recordingController.IsRecording(cs.DeviceID())
		res = append(res, status)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"active_cameras": len(streams),
		"cameras":        res,
	})
}

func handleRecord(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
		return
	}

	var req RecordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Erro ao decodificar JSON", http.StatusBadRequest)
		return
	}

	if req.DeviceID == "" {
		req.DeviceID = "esp32-cam-01"
	}

	if req.VideoURL != "" {
		streamManager.Register(req.DeviceID, req.VideoURL)
	}

	proxyStreamURL := fmt.Sprintf("http://127.0.0.1:%s/live?device_id=%s", serverPort, req.DeviceID)

	status, err := recordingController.TriggerEvent(req.DeviceID, proxyStreamURL)
	if err != nil {
		log.Printf("[Media Gateway ERROR] Falha ao processar evento de gravação: %v\n", err)
		http.Error(w, fmt.Sprintf("Erro ao iniciar gravação: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":    status,
		"device_id": req.DeviceID,
	})
}

func handleListRecordings(w http.ResponseWriter, r *http.Request) {
	recordings := metadataStore.GetAll()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(recordings)
}

func handleServeRecording(w http.ResponseWriter, r *http.Request) {
	fileName := strings.TrimPrefix(r.URL.Path, "/recordings/")
	fileName = filepath.Clean(fileName)

	if strings.Contains(fileName, "..") || strings.HasPrefix(fileName, "/") {
		http.Error(w, "Acesso inválido", http.StatusBadRequest)
		return
	}

	fullPath := filepath.Join(recordingsDir, fileName)
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		http.Error(w, "Gravação não encontrada", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "video/mp4")
	http.ServeFile(w, r, fullPath)
}

func extractDeviceID(r *http.Request, prefixes ...string) string {
	devID := r.URL.Query().Get("device_id")
	if devID != "" {
		return devID
	}
	path := r.URL.Path
	for _, p := range prefixes {
		if strings.HasPrefix(path, p) {
			trimmed := strings.TrimPrefix(path, p)
			trimmed = strings.Trim(trimmed, "/")
			if trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}

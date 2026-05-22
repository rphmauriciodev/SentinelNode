package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/rphmauriciodev/SentinelNode/media-gateway/config"
)

type RecordRequest struct {
	DeviceID        string `json:"device_id"`
	VideoURL        string `json:"video_url"`
	DurationSeconds int    `json:"duration_seconds"`
}

var (
	recordingsDir    string
	activeRecordings = make(map[string]bool)
	recordingsMu     sync.Mutex
)

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Erro ao carregar configuração: %v", err)
	}

	recordingsDir = cfg.RecordingsDir

	mux := http.NewServeMux()

	mux.HandleFunc("/record", handleRecord)

	server := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.Port),
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		log.Println("Iniciando servidor na porta", server.Addr)
		if err := server.ListenAndServe(); err != nil {
			log.Fatalf("Erro ao iniciar servidor: %v", err)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan

	log.Println("Servidor desligado com sucesso.")
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

	recordingsMu.Lock()
	if activeRecordings[req.DeviceID] {
		recordingsMu.Unlock()
		log.Printf("[Media Gateway] Gravação já em andamento para o dispositivo %s. Ignorando pedido duplicado.\n", req.DeviceID)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status": "already_recording", "device_id": "%s"}`, req.DeviceID)
		return
	}
	activeRecordings[req.DeviceID] = true
	recordingsMu.Unlock()

	log.Printf("Pedido de gravação recebido: Device=%s, URL=%s, Duração=%ds\n",
		req.DeviceID, req.VideoURL, req.DurationSeconds)

	go func() {
		recordVideo(req)

		recordingsMu.Lock()
		delete(activeRecordings, req.DeviceID)
		recordingsMu.Unlock()
	}()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status": "recording_started", "device_id": "%s"}`,
		req.DeviceID)
}

func recordVideo(req RecordRequest) {
	err := os.MkdirAll(recordingsDir, os.ModePerm)
	if err != nil {
		log.Printf("Erro ao criar a pasta recordings: %v\n", err)
		return
	}

	outputPath := fmt.Sprintf("%s/%s_%d.mp4", recordingsDir, req.DeviceID, time.Now().Unix())

	isHttp := len(req.VideoURL) > 7 && req.VideoURL[:7] == "http://"
	isRtsp := len(req.VideoURL) > 7 && req.VideoURL[:7] == "rtsp://"

	if isRtsp || isHttp {
		log.Printf("[FFmpeg] Tentando gravar de CAMERA REAL (%s): %s\n", req.DeviceID, req.VideoURL)

		var cmd *exec.Cmd
		if isRtsp {
			cmd = exec.Command("ffmpeg",
				"-y",
				"-rtsp_transport", "tcp",
				"-i", req.VideoURL,
				"-t", fmt.Sprintf("%d", req.DurationSeconds),
				"-c", "copy",
				outputPath,
			)
		} else {
			// Para stream HTTP MJPEG da ESP32-CAM, precisamos transcodificar para H.264
			cmd = exec.Command("ffmpeg",
				"-y",
				"-i", req.VideoURL,
				"-t", fmt.Sprintf("%d", req.DurationSeconds),
				"-c:v", "libx264",
				"-pix_fmt", "yuv420p",
				outputPath,
			)
		}

		output, err := cmd.CombinedOutput()
		if err == nil {
			log.Printf("[FFmpeg] Sucesso! Vídeo da câmera real salvo em: %s\n", outputPath)
			return
		}

		log.Printf("[FFmpeg] Falha ao conectar ou gravar da câmera real. Ativando simulador local... Erro: %v\n", err)
		log.Printf("Console do FFmpeg: %s\n", string(output))
	}

	log.Printf("[FFmpeg] Iniciando gerador local de imagens para teste (Câmera simulada)...\n")

	cmdFallback := exec.Command("ffmpeg",
		"-y",
		"-f", "lavfi",
		"-i", "testsrc=size=640x480:rate=30",
		"-t", fmt.Sprintf("%d", req.DurationSeconds),
		"-c:v", "libx264",
		"-pix_fmt", "yuv420p",
		outputPath,
	)

	output, err := cmdFallback.CombinedOutput()
	if err != nil {
		log.Printf("[FFmpeg ERROR] Falha no fallback: %v. Output: %s\n", err, string(output))
		return
	}

	log.Printf("[FFmpeg] Gravação simulada concluída! Vídeo salvo em: %s\n", outputPath)
}

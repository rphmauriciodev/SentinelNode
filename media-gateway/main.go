package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
)

type RecordRequest struct {
	DeviceID        string `json:"device_id"`
	VideoURL        string `json:"video_url"`
	DurationSeconds int    `json:"duration_seconds"`
}

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("/record", handleRecord)

	server := &http.Server{
		Addr:         ":8080",
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

	log.Printf("Pedido de gravação recebido: Device=%s, URL=%s, Duração=%ds\n",
		req.DeviceID, req.VideoURL, req.DurationSeconds)

	go recordVideo(req)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status": "recording_started", "device_id": "%s"}`,
		req.DeviceID)
}

func recordVideo(req RecordRequest) {
	err := os.MkdirAll("recordings", os.ModePerm)
	if err != nil {
		log.Printf("Erro ao criar a pasta recordings: %v\n", err)
		return
	}

	outputPath := fmt.Sprintf("recordings/%s_%d.mp4", req.DeviceID, time.Now().Unix())

	if len(req.VideoURL) > 7 && req.VideoURL[:7] == "rtsp://" && req.VideoURL != "rtsp://localhost:8554/live/edge-agent-01" {
		log.Printf("[FFmpeg] Tentando gravar de CAMERA REAL RTSP: %s\n", req.VideoURL)

		cmd := exec.Command("ffmpeg",
			"-y",
			"-rtsp_transport", "tcp",
			"-i", req.VideoURL,
			"-t", fmt.Sprintf("%d", req.DurationSeconds),
			"-c", "copy",
			outputPath,
		)

		output, err := cmd.CombinedOutput()
		if err == nil {
			log.Printf("[FFmpeg] Sucesso! Vídeo da câmera real salvo em: %s\n", outputPath)
			return
		}

		log.Printf("[FFmpeg] Falha ao conectar na câmera real. Ativando simulador local... Erro: %v\n", err)
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

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
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
	log.Printf("[FFmpeg] Iniciando gravação de %s por %d segundos...", req.VideoURL, req.DurationSeconds)

	time.Sleep(time.Duration(req.DurationSeconds) * time.Second)

	log.Printf("[FFmpeg] Gravação finalizada para %s", req.DeviceID)

}

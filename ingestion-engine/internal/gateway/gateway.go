package gateway

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"

	"github.com/rphmauriciodev/SentinelNode/ingestion-engine/internal/models"
)

func TriggerRecording(gatewayURL string, deviceID string, videoURL string) {
	log.Printf("[API Media Gateway] Enviando comando de gravação de 10s para %s...\n", deviceID)

	reqBody := models.RecordRequest{
		DeviceID:        deviceID,
		VideoURL:        videoURL,
		DurationSeconds: 10,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		log.Printf("Erro ao serializar payload para o media-gateway: %v\n", err)
		return
	}

	log.Printf("[HTTP REST] Enviando solicitação de gravação para o Media Gateway (%s)...\n", gatewayURL)

	response, err := http.Post(gatewayURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("Erro ao enviar solicitação de gravação para o gateway: %v\n", err)
		return
	}
	defer response.Body.Close()

	log.Printf("[API Media Gateway] Resposta recebida: %s\n", response.Status)

	var result map[string]interface{}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		log.Printf("Erro ao decodificar JSON da resposta do gateway: %v\n", err)
		return
	}

	log.Printf("[API Media Gateway] Status da gravação retornado: %s\n", result["status"])
}

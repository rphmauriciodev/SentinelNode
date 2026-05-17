package worker

import (
	"log"
	"time"

	"github.com/rphmauriciodev/SentinelNode/ingestion-engine/internal/database"
	"github.com/rphmauriciodev/SentinelNode/ingestion-engine/internal/models"
)

var EventQueue chan models.EventMessage

func Init(bufferSize int) {
	EventQueue = make(chan models.EventMessage, bufferSize)
}

func Start() {
	log.Println("👷 Worker de persistência de banco de dados iniciado em background...")
	
	for event := range EventQueue {
		backoff := 1 * time.Second
		maxBackoff := 30 * time.Second

		for {
			err := database.InsertEvent(event)
			if err == nil {
				log.Printf("💾 [BANCO] Evento do dispositivo [%s] Salvo no banco de dados!\n", event.DeviceID)
				break 
			}

			log.Printf("❌ [BANCO ERROR] Falha ao salvar evento do dispositivo [%s]: %v. Tentando novamente em %v...\n",
				event.DeviceID, err, backoff)
			
			time.Sleep(backoff)

			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
}

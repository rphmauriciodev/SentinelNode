package database

import (
	"database/sql"
	"log"

	_ "github.com/lib/pq"
	"github.com/rphmauriciodev/SentinelNode/ingestion-engine/internal/models"
)

var DB *sql.DB

func Init(connStr string) {
	var err error
	DB, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Fatalf("Erro crítico ao abrir conexão com banco de dados: %v", err)
	}

	err = DB.Ping()
	if err != nil {
		log.Printf("⚠️ Aviso: Banco de dados inicial offline. O worker tentará reconectar em background. %v\n", err)
	}

	if err := EnsureTable(); err != nil {
		log.Printf("⚠️ Aviso: Não foi possível criar/verificar a tabela no banco (banco offline?): %v\n", err)
	} else {
		log.Println("💾 Tabela 'security_events' verificada/criada com sucesso.")
	}
}

func EnsureTable() error {
	if DB == nil {
		return nil
	}
	query := `
	CREATE TABLE IF NOT EXISTS security_events (
		id SERIAL PRIMARY KEY,
		device_id VARCHAR(50) NOT NULL,
		event_type VARCHAR(50) NOT NULL,
		video_url TEXT NOT NULL,
		timestamp TIMESTAMP NOT NULL
	);`
	_, err := DB.Exec(query)
	return err
}

func InsertEvent(event models.EventMessage) error {
	if DB == nil {
		return nil
	}
	query := `INSERT INTO security_events (device_id, event_type, video_url, timestamp) VALUES ($1, $2, $3, $4)`
	_, err := DB.Exec(query, event.DeviceID, event.EventType, event.VideoURL, event.Timestamp)
	if err != nil {
		if ensureErr := EnsureTable(); ensureErr == nil {
			_, err = DB.Exec(query, event.DeviceID, event.EventType, event.VideoURL, event.Timestamp)
		}
	}
	return err
}

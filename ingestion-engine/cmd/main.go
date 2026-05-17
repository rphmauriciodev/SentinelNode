package main

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/rphmauriciodev/SentinelNode/ingestion-engine/internal/database"
	"github.com/rphmauriciodev/SentinelNode/ingestion-engine/internal/gateway"
	"github.com/rphmauriciodev/SentinelNode/ingestion-engine/internal/models"
	"github.com/rphmauriciodev/SentinelNode/ingestion-engine/internal/protocol"
	"github.com/rphmauriciodev/SentinelNode/ingestion-engine/internal/worker"
)

func main() {
	log.Println("--- SentinelNode: Ingestion Engine ---")

	tlsConfig, err := NewTLSConfig()
	if err != nil {
		log.Fatalf("Erro ao criar configuração TLS: %v", err)
	}

	worker.Init(100)

	connStr := "host=localhost port=5432 user=sentinel_user password=sentinel_password dbname=sentinel_db sslmode=disable"
	database.Init(connStr)
	defer database.DB.Close()

	go worker.Start()

	opts := mqtt.NewClientOptions()
	opts.AddBroker("ssl://localhost:8883")
	opts.SetClientID("ingestion-engine-01")
	opts.SetKeepAlive(60 * time.Second)
	opts.SetPingTimeout(1 * time.Second)
	opts.SetTLSConfig(tlsConfig)

	opts.OnConnect = func(client mqtt.Client) {
		log.Println("Backend conectado ao broker MQTT com sucesso!")
		qos := byte(1)

		hbTopic := "sentinel/devices/+/heartbeat"
		if token := client.Subscribe(hbTopic, qos, handleHeartbeat); token.Wait() && token.Error() != nil {
			log.Printf("Erro ao se inscrever em heartbeats [%s]: %v\n", hbTopic, token.Error())
		} else {
			log.Printf("Inscrito em heartbeats: %s\n", hbTopic)
		}

		eventTopic := "sentinel/devices/+/events"
		if token := client.Subscribe(eventTopic, qos, handleEvent); token.Wait() && token.Error() != nil {
			log.Printf("Erro ao se inscrever em eventos [%s]: %v\n", eventTopic, token.Error())
		} else {
			log.Printf("Inscrito em eventos de seguranca: %s\n", eventTopic)
		}
	}

	opts.OnConnectionLost = func(client mqtt.Client, err error) {
		log.Printf("Conexão perdida com o broker: %v. Tentando reconectar...\n", err)
	}

	client := mqtt.NewClient(opts)
	log.Println("Iniciando conexão com Broker MQTT...")
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		log.Fatalf("Erro crítico ao conectar ao Broker MQTT: %v", token.Error())
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	log.Println("Ingestion Engine totalmente operacional. Pressione Ctrl+C para encerrar.")
	<-sigChan

	log.Println("Encerrando Ingestion Engine...")
	client.Disconnect(250)
	log.Println("Backend finalizado com sucesso.")
}

func handleHeartbeat(client mqtt.Client, msg mqtt.Message) {
	_, payload, err := protocol.Decode(msg.Payload())
	if err != nil {
		log.Printf("Erro ao decodificar binário do heartbeat: %v\n", err)
		return
	}

	hb, ok := payload.(models.Heartbeat)

	if !ok {
		log.Println("Erro: payload retornado não é models.Heartbeat")
		return
	}

	log.Printf("[HEARTBEAT] Dispositivo [%s] saudável. Firmware: %s. Status: %s\n",
		hb.DeviceID, hb.FirmwareVersion, hb.Status)
}

func handleEvent(client mqtt.Client, msg mqtt.Message) {

	_, payload, err := protocol.Decode(msg.Payload())

	if err != nil {
		log.Printf("Erro ao decodificar binário do evento: %v\n", err)
		return
	}

	event, ok := payload.(models.EventMessage)

	if !ok {
		log.Println("Erro: payload retornado não é models.EventMessage")
		return
	}

	log.Printf("\n=== NOVO ALERTA DE SEGURANÇA RECEBIDO ===")
	log.Printf("Dispositivo: %s | Evento: %s | Horario: %v",
		event.DeviceID, event.EventType, event.Timestamp.Format("15:04:05"))
	log.Printf("=========================================\n")

	select {
	case worker.EventQueue <- event:
	default:
		log.Printf("AVISO BACKPRESSURE: Fila cheia! Evento da câmera [%s] foi descartado.\n", event.DeviceID)
	}

	if event.EventType == "motion_detected" {
		gatewayURL := "http://localhost:8080/record"
		go gateway.TriggerRecording(gatewayURL, event.DeviceID, event.VideoURL)
	}
}

func NewTLSConfig() (*tls.Config, error) {
	caCert, err := os.ReadFile("../certs/ca.crt")
	if err != nil {
		return nil, err
	}

	rootPool := x509.NewCertPool()
	if !rootPool.AppendCertsFromPEM(caCert) {
		return nil, errors.New("failed to append CA certificate")
	}

	cert, err := tls.LoadX509KeyPair("../certs/client.crt", "../certs/client.key")
	if err != nil {
		return nil, err
	}

	return &tls.Config{
		RootCAs:            rootPool,
		Certificates:       []tls.Certificate{cert},
		InsecureSkipVerify: true,
	}, nil
}

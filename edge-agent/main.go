package main

import (
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type Heartbeat struct {
	DeviceID        string    `json:"device_id"`
	Timestamp       time.Time `json:"timestamp"`
	Status          string    `json:"status"`
	FirmwareVersion string    `json:"firmware_version"`
}

func main() {
	opts := mqtt.NewClientOptions()
	opts.AddBroker("tcp://localhost:1883")
	opts.SetClientID("edge-agent-01")
	opts.SetKeepAlive(60 * time.Second)
	opts.SetPingTimeout(1 * time.Second)

	opts.OnConnect = func(client mqtt.Client) {
		log.Println("Conectado ao broker MQTT com sucesso!")
	}
	opts.OnConnectionLost = func(client mqtt.Client, err error) {
		log.Printf("Conexão perdida com o broker: %v\n", err)
	}

	client := mqtt.NewClient(opts)

	log.Println("Tentando conectar ao broker...")
	token := client.Connect()
	if token.Wait() && token.Error() != nil {
		log.Fatalf("Erro ao conectar ao broker: %v", token.Error())
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	log.Println("Agente rodando. Pressione Ctrl+C para encerrar.")

	doneChan := make(chan struct{})

	go func() {
		for {
			select {
			case <-ticker.C:
				payload := Heartbeat{
					DeviceID:        "edge-agent-01",
					Timestamp:       time.Now(),
					Status:          "active",
					FirmwareVersion: "1.0.0",
				}

				jsonData, err := json.Marshal(payload)
				if err != nil {
					log.Printf("Erro ao serializar payload: %v\n", err)
					continue
				}

				topic := "sentinel/devices/edge-agent-01/heartbeat"

				token := client.Publish(topic, 1, false, jsonData)
				if token.Wait() && token.Error() != nil {
					log.Printf("Erro ao enviar payload: %v\n", token.Error())
				}

				log.Printf("Payload enviado para o broker: %s\n", string(jsonData))

			case <-doneChan:
				log.Println("Loop de heartbeats finalizado.")
				return
			}
		}
	}()

	<-sigChan

	log.Println("Encerrando agente de forma graciosa...")

	close(doneChan)

	client.Disconnect(250)
	log.Println("Agente finalizado.")
}

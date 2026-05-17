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
	opts.SetClientID("ingestion-engine-01")
	opts.SetKeepAlive(60 * time.Second)
	opts.SetPingTimeout(1 * time.Second)

	opts.OnConnect = func(client mqtt.Client) {
		log.Println("Backend conectado ao broker MQTT com sucesso!")

		topic := "sentinel/devices/+/heartbeat"
		qos := byte(1)

		token := client.Subscribe(topic, qos, handleHeartbeat)
		if token.Wait() && token.Error() != nil {
			log.Printf("Erro ao se inscrever no tópico [%s]: %v\n", topic, token.Error())
			return
		}
		log.Printf("Inscrito com sucesso no tópico: %s\n", topic)
	}

	opts.OnConnectionLost = func(client mqtt.Client, err error) {
		log.Printf("Conexão perdida com o broker: %v. Tentando reconectar...\n", err)
	}

	client := mqtt.NewClient(opts)
	log.Println("Backend iniciando conexão...")
	token := client.Connect()
	if token.Wait() && token.Error() != nil {
		log.Fatalf("Erro crítico ao conectar ao broker: %v", token.Error())
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	log.Println("Ingestion Engine rodando. Aguardando dados dos dispositivos. Pressione Ctrl+C para encerrar.")
	<-sigChan

	log.Println("Encerrando Ingestion Engine...")
	client.Disconnect(250)
	log.Println("Backend desligado.")
}

func handleHeartbeat(client mqtt.Client, msg mqtt.Message) {
	log.Printf("[Mensagem Recebida] Tópico: %s\n", msg.Topic())

	var hb Heartbeat
	err := json.Unmarshal(msg.Payload(), &hb)
	if err != nil {
		log.Printf("Erro ao decodificar JSON do heartbeat: %v (Payload bruto: %s)\n", err, string(msg.Payload()))
		return
	}

	log.Printf(">> Dispositivo [%s] está saudável (%s). Firmware: %s. Enviado em: %v\n",
		hb.DeviceID, hb.Status, hb.FirmwareVersion, hb.Timestamp.Format(time.RFC3339))
}

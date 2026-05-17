package main

import (
	"bufio"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/rphmauriciodev/SentinelNode/edge-agent/config"
	"github.com/rphmauriciodev/SentinelNode/edge-agent/internal/protocol"
)

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Erro ao carregar configuração: %v", err)
	}

	tlsConfig, err := NewTLSConfig()
	if err != nil {
		log.Fatalf("Erro ao criar configuração TLS: %v", err)
	}

	opts := mqtt.NewClientOptions()
	opts.AddBroker(cfg.MQTTBroker)
	opts.SetClientID(cfg.MQTTClientID)
	opts.SetKeepAlive(60 * time.Second)
	opts.SetPingTimeout(5 * time.Second)
	opts.SetTLSConfig(tlsConfig)

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

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	log.Println("Agente rodando. Pressione Ctrl+C para encerrar.")

	doneChan := make(chan struct{})

	go func() {
		for {
			select {
			case <-ticker.C:
				payload := protocol.EncodeHeartbeat(
					cfg.DeviceID,
					time.Now(),
					"active",
					"1.0.0",
				)

				topic := fmt.Sprintf("sentinel/devices/%s/heartbeat", cfg.DeviceID)

				token := client.Publish(topic, 1, false, payload)
				if token.Wait() && token.Error() != nil {
					log.Printf("Erro ao enviar payload: %v\n", token.Error())
				}

				log.Printf("Payload enviado para o broker: %x\n", payload)

			case <-doneChan:
				log.Println("Loop de heartbeats finalizado.")
				return
			}
		}
	}()

	go func() {
		scanner := bufio.NewScanner(os.Stdin)

		for scanner.Scan() {
			event := protocol.EncodeEvent(
				cfg.DeviceID,
				time.Now(),
				"motion_detected",
				fmt.Sprintf("rtsp://localhost:8554/live/%s", cfg.DeviceID),
			)

			topic := fmt.Sprintf("sentinel/devices/%s/events", cfg.DeviceID)
			log.Println("Publicando alerta de movimento...")

			token := client.Publish(topic, 1, false, event)
			if token.Wait() && token.Error() != nil {
				log.Printf("Erro ao enviar payload: %v\n", token.Error())
			}

			log.Printf("Payload enviado para o broker: %x\n", event)
			log.Printf("Alerta binário enviado (%d bytes). URL: %s\n", len(event), "rtsp://localhost:8554/live/edge-agent-01")
		}
	}()

	<-sigChan

	log.Println("Encerrando agente de forma graciosa...")

	close(doneChan)

	client.Disconnect(250)
	log.Println("Agente finalizado.")
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

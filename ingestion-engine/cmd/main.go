package main

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/rphmauriciodev/SentinelNode/ingestion-engine/internal/config"
	"github.com/rphmauriciodev/SentinelNode/ingestion-engine/internal/database"
	"github.com/rphmauriciodev/SentinelNode/ingestion-engine/internal/gateway"
	"github.com/rphmauriciodev/SentinelNode/ingestion-engine/internal/models"
	"github.com/rphmauriciodev/SentinelNode/ingestion-engine/internal/protocol"
	"github.com/rphmauriciodev/SentinelNode/ingestion-engine/internal/worker"
)

var gatewayURL string
var durationSeconds int

var activeStreams sync.Map
var pirToCameraMap = map[string][]string{
	"esp32-pir-01": {"esp32-cam-01"},
}

func loadPirMappings() map[string][]string {
	mapping := map[string][]string{
		"esp32-pir-01": {"esp32-cam-01"},
	}
	envMapping := os.Getenv("PIR_CAMERA_MAPPING")
	if envMapping == "" {
		return mapping
	}
	pairs := strings.Split(envMapping, ";")
	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		parts := strings.SplitN(pair, ":", 2)
		if len(parts) == 2 {
			pirID := strings.TrimSpace(parts[0])
			camList := strings.Split(parts[1], ",")
			var cams []string
			for _, c := range camList {
				c = strings.TrimSpace(c)
				if c != "" {
					cams = append(cams, c)
				}
			}
			if pirID != "" && len(cams) > 0 {
				mapping[pirID] = cams
			}
		}
	}
	return mapping
}
func main() {
	duration := flag.Int("duration", 60, "Duração da gravação em segundos")
	flag.Parse()
	durationSeconds = *duration
	pirToCameraMap = loadPirMappings()

	log.Println("--- SentinelNode: Ingestion Engine ---")
	log.Printf("Mapeamento PIR -> Câmeras: %+v\n", pirToCameraMap)
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Erro ao carregar configuração: %v", err)
	}

	gatewayURL = cfg.MediaGatewayURL

	tlsConfig, err := NewTLSConfig()
	if err != nil {
		log.Fatalf("Erro ao criar configuração TLS: %v", err)
	}

	worker.Init(100)

	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBPassword, cfg.DBName, cfg.DBSslMode)
	database.Init(connStr)
	defer database.DB.Close()

	go worker.Start()

	opts := mqtt.NewClientOptions()
	opts.AddBroker(cfg.MQTTBroker)
	opts.SetClientID(cfg.MQTTClientID)
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

	if hb.StreamURL != "" {
		activeStreams.Store(hb.DeviceID, hb.StreamURL)
		go gateway.RegisterStream(gatewayURL, hb.DeviceID, hb.StreamURL)
	}

	log.Printf("[HEARTBEAT] Dispositivo [%s] saudável. Firmware: %s. Status: %s. StreamURL: %s\n",
		hb.DeviceID, hb.FirmwareVersion, hb.Status, hb.StreamURL)
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
	var targetCameras []string
	if targets, exists := pirToCameraMap[event.DeviceID]; exists && len(targets) > 0 {
		targetCameras = targets
	} else {
		targetCameras = []string{event.DeviceID}
	}

	log.Printf("\n=== NOVO ALERTA DE SEGURANÇA RECEBIDO ===")
	log.Printf("Dispositivo: %s | Câmeras Alvo: %v | Evento: %s | Horario: %v",
		event.DeviceID, targetCameras, event.EventType, event.Timestamp.Format("15:04:05"))
	log.Printf("=========================================\n")

	for _, targetCamID := range targetCameras {
		streamURL := event.VideoURL
		if streamURL == "" {
			if val, found := activeStreams.Load(targetCamID); found {
				streamURL = val.(string)
				log.Printf("[EVENT MAP] Associando evento de [%s] à stream ativa de [%s]: %s\n",
					event.DeviceID, targetCamID, streamURL)
			} else {
				log.Printf("[EVENT MAP WARNING] Evento de [%s] (alvo [%s]) disparou, mas não possui stream ativa no cache!\n",
					event.DeviceID, targetCamID)
			}
		}

		camEvent := event
		camEvent.DeviceID = targetCamID
		camEvent.VideoURL = streamURL

		select {
		case worker.EventQueue <- camEvent:
		default:
			log.Printf("AVISO BACKPRESSURE: Fila cheia! Evento da câmera [%s] foi descartado.\n", targetCamID)
		}

		if event.EventType == "motion_detected" && streamURL != "" {
			go gateway.TriggerRecording(gatewayURL, targetCamID, streamURL, durationSeconds)
		}
	}
}

func resolveCertPath(relPath string) string {
	candidates := []string{
		filepath.Join("..", relPath),
		relPath,
		filepath.Join(".", relPath),
	}
	if envDir := os.Getenv("CERTS_DIR"); envDir != "" {
		candidates = append([]string{filepath.Join(envDir, filepath.Base(relPath))}, candidates...)
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return filepath.Join("..", relPath)
}

func NewTLSConfig() (*tls.Config, error) {
	caPath := resolveCertPath("certs/ca.crt")
	clientCrtPath := resolveCertPath("certs/client.crt")
	clientKeyPath := resolveCertPath("certs/client.key")

	caCert, err := os.ReadFile(caPath)
	if err != nil {
		return nil, fmt.Errorf("falha ao ler CA cert (%s): %w", caPath, err)
	}

	rootPool := x509.NewCertPool()
	if !rootPool.AppendCertsFromPEM(caCert) {
		return nil, errors.New("failed to append CA certificate")
	}

	cert, err := tls.LoadX509KeyPair(clientCrtPath, clientKeyPath)
	if err != nil {
		return nil, fmt.Errorf("falha ao carregar par de chaves do cliente (%s, %s): %w", clientCrtPath, clientKeyPath, err)
	}

	return &tls.Config{
		RootCAs:            rootPool,
		Certificates:       []tls.Certificate{cert},
		InsecureSkipVerify: true,
	}, nil
}

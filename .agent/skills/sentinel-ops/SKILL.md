---
name: sentinel-ops
description: >-
  Procedimentos operacionais, rotinas de build, testes e diagnóstico do ecossistema SentinelNode 2.0.
---

# Procedimentos Operacionais do SentinelNode 2.0

## 1. Como Rodar a Suíte Completa de Testes

```bash
# Validação do Media Gateway (10 cenários obrigatórios, stream e recorder)
cd media-gateway && go test -v ./...

# Validação do Ingestion Engine (decodificação de protocolo binário)
cd ../ingestion-engine && go test -v ./...

# Validação do Edge Agent (serialização de protocolo binário)
cd ../edge-agent && go test -v ./...
```

## 2. Como Subir os Serviços Localmente

### Passo 1: Infraestrutura (Mosquitto & PostgreSQL)
```bash
docker compose up -d
```
* PostgreSQL roda mapeado na porta `5435` do host (`localhost:5435`).
* Mosquitto MQTT roda nas portas `1883`, `8883` (mTLS) e `9001` (WebSockets).

### Passo 2: Media Gateway
```bash
cd media-gateway
go run main.go
```
* Dashboard Web: `http://localhost:8080/`
* Stream ao Vivo: `http://localhost:8080/live?device_id=esp32-cam-01`
* Snapshot JPEG: `http://localhost:8080/snapshot?device_id=esp32-cam-01`

### Passo 3: Ingestion Engine
```bash
cd ingestion-engine
go run cmd/main.go
```

## 3. Emulação de Câmera e Testes sem Hardware

Se não houver um ESP32-CAM físico disponível:
```bash
# Executar o emulador em Python
cd firmware/ESP-CAM
python3 emulator.py
```
O emulador transmitirá MJPEG na porta `8087` e enviará heartbeats MQTT mTLS para o Mosquitto.

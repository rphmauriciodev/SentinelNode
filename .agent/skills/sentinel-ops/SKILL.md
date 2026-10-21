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

## 2. Como Subir os Serviços via Docker (Recomendado para VM Local)

### Subida Completa (Broker, DB, Media Gateway e Ingestion Engine)
```bash
docker compose up -d
# ou: make up
```
* Media Gateway (Web UI e Stream): `http://<IP_VM>:8080/`
* Broker MQTT mTLS: `<IP_VM>:8883`
* PostgreSQL: `<IP_VM>:5435`

### Subida Completa + Emulador de Câmera ESP32-CAM
```bash
docker compose --profile emulator up -d
# ou: make dev
```
* Emulador de Stream: `http://<IP_VM>:8087/stream`
* Disparo manual PIR: `curl -X POST http://<IP_VM>:8087/pir/trigger`

## 3. Como Subir os Serviços Nativamente (Sem Docker para Go)

### Passo 1: Infraestrutura (Mosquitto & PostgreSQL)
```bash
make docker-infra
```

### Passo 2: Iniciar Serviços Nativos
```bash
make host-up
# ou: make host-dev (inclui emulador)
```
## 3. Emulação de Câmera e Testes sem Hardware

Se não houver um ESP32-CAM físico disponível:
```bash
# Executar o emulador em Python
cd firmware/ESP-CAM
python3 emulator.py
```
O emulador transmitirá MJPEG na porta `8087` e enviará heartbeats MQTT mTLS para o Mosquitto.

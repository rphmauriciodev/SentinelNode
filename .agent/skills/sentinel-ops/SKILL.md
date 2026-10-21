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

### Opção Standalone (Sem Clonar o Repositório na VM)
Em uma VM limpa com apenas Docker e Docker Compose instalados:
```bash
mkdir -p sentinelnode && cd sentinelnode
curl -sO https://raw.githubusercontent.com/rphmauriciodev/SentinelNode/main/docker-compose.yaml
docker compose up -d
# ou com o emulador de hardware:
docker compose --profile emulator up -d
```
* Baixa automaticamente as imagens oficiais do Docker Hub (`rphmauriciodev/sentinel-*`).
* Cria automaticamente a pasta `./certs/` com os certificados mTLS e `./recordings/` com os MP4s gravados.

### Opção com Repositório Clonado
```bash
# Subida completa da stack
make up

# Subida completa + emulador de câmera
make dev

# Parar containers
make down
```
* Media Gateway (Web UI e Stream): `http://<IP_VM>:8080/`
* Broker MQTT mTLS: `<IP_VM>:8883`
* PostgreSQL: `<IP_VM>:5435`
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

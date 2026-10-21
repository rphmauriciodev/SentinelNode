# SENTINELNODE 2.0 - CONTEXTO E DIRETRIZES DE AGENTE

Este repositório contém o **SentinelNode 2.0**, um ecossistema de vigilância pessoal e monitoramento IoT industrial baseado em ESP32-CAM, comunicação MQTT segura via mTLS e backend em Go.

---

## 1. Visão Geral da Arquitetura

```
[ ESP32-CAM / ESP-PIR / Emulador ]
       │                      │
       │ (1 Conexão HTTP)     │ (mTLS :8883)
       ▼                      ▼
[ Media Gateway :8080 ]   [ Mosquitto Broker ]
       │                      │
       │                      ▼
       │              [ Ingestion Engine ]
       │                      │
       │                      ▼
       │                [ PostgreSQL ]
       ▼
 [ Broadcaster em Memória ]
       ├──> Live Web UI (:8080)
       ├──> N Espectadores Simultâneos (GET /live)
       └──> Recording Controller (Sliding Window 30s) -> Arquivos MP4
```

---

## 2. Invariantes Críticos do Sistema

Ao interagir ou modificar este repositório, você DEVE respeitar rigorosamente as seguintes regras arquiteturais:

1. **Capping de 1 Conexão no ESP32-CAM:**
   * O firmware da ESP32-CAM (`HttpStreamService.cpp`) rejeita com HTTP 403 qualquer conexão secundária.
   * **NUNCA** faça conexões diretas da UI ou do FFmpeg para a câmera física.
   * Toda e qualquer visualização e gravação DEVE passar pelo `stream.Broadcaster` centralizado do `media-gateway` (`http://127.0.0.1:8080/live?device_id=...`).
2. **Gravação Orientada a Eventos PIR:**
   * O fluxo contínuo ao vivo fica em memória para os visualizadores.
   * Nenhuma gravação deve ser persistida em disco sem que haja evento de detecção de movimento válido recebido via MQTT.
3. **Janela Deslizante (Sliding Window de 30s):**
   * O início da gravação ocorre no primeiro evento PIR.
   * Cada novo evento recebido durante uma gravação ativa deve renovar o temporizador de tolerância (30 segundos).
   * Não fragmente o vídeo em múltiplos arquivos para o mesmo período contínuo de atividade.
4. **Finalização Graciosa do FFmpeg:**
   * O encerramento de gravações deve enviar o comando `q` pelo `stdin` do processo (ou `SIGINT`), aguardando a gravação correta do átomo `moov` (MP4) para evitar corrupção de arquivos de vídeo.
5. **Protocolo Binário de Rede:**
   * Mensagens MQTT utilizam Magic Number `0x53 0x4E` ("SN"), timestamp `uint64` Big-Endian, tipo `0x01` (Heartbeat) ou `0x02` (Evento PIR).
   * MQTT **NÃO** deve ser utilizado para transportar frames de vídeo.
6. **Autonomia Local:**
   * O sistema opera 100% na rede local (LAN), sem dependências externas de nuvem ou internet.

---

## 3. Módulos do Repositório

* `media-gateway/`: Gateway de vídeo Go (porta 8080).
  * `internal/stream/`: Ingestão contínua da câmera e Broadcaster para múltiplos clientes.
  * `internal/recorder/`: Controlador de gravação com sliding window (30s) e catálogo de metadados.
  * `internal/web/`: Painel web responsivo (dashboard HTML/CSS/JS) com player ao vivo e histórico de gravações.
* `ingestion-engine/`: Motor de eventos Go.
  * Consome tópicos MQTT (`sentinel/devices/+/heartbeat` e `sentinel/devices/+/events`) via mTLS 8883.
  * Worker com backoff exponencial para persistência no PostgreSQL.
  * Registra câmeras ativas dinamicamente no `media-gateway`.
* `firmware/`:
  * `src/main.cpp`: Código modular C++ do ESP32-CAM (FreeRTOS, Watchdog, mTLS, HTTP MJPEG :80).
  * `ESP-PIR/`: Firmware para sensor PIR em ESP32 dedicado.
  * `ESP-CAM/emulator.py`: Emulador de câmera em Python.
* `edge-agent/`: Simulador de agente de borda em Go.
* `docker-compose.yaml`: Containers Mosquitto (1883, 8883, 9001) e PostgreSQL 15 (porta 5435 no host).

---

## 4. Endpoints da API do Media Gateway (:8080)

| Endpoint | Método | Descrição |
| :--- | :---: | :--- |
| `/` ou `/dashboard` | `GET` | Interface web de vigilância pessoal. |
| `/live?device_id=...` | `GET` | Stream ao vivo HTTP MJPEG (suporta múltiplos clientes simultâneos). |
| `/snapshot?device_id=...` | `GET` | Retorna o frame JPEG mais recente. |
| `/status` | `GET` | Métricas operacionais em JSON (FPS, frames, espectadores, status). |
| `/record` | `POST` | Aciona ou estende gravação PIR com janela deslizante de 30s. |
| `/api/recordings` | `GET` | Lista de metadados das gravações finalizadas e em andamento. |
| `/recordings/<arquivo>.mp4` | `GET` | Serve arquivo MP4 com suporte a busca e streaming HTML5. |
| `/stream/register` | `POST` | Registra dinamicamente uma URL de stream de câmera. |

---

## 5. Como Testar e Validar

```bash
# Testes do Media Gateway (Stream, Recorder e Integração de todos os 10 cenários)
cd media-gateway && go test -v ./...

# Testes do Ingestion Engine
cd ../ingestion-engine && go test -v ./...

# Testes do Edge Agent
cd ../edge-agent && go test -v ./...
```

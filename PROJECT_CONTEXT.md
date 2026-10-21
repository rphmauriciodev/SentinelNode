# SentinelNode 2.0 - Contexto da Aplicação

Documento de referência consolidado para inicialização e carregamento rápido de contexto por agentes de IA e desenvolvedores.

---

## 1. Visão Geral e Propósito

O **SentinelNode 2.0** é um ecossistema de vigilância pessoal e monitoramento de borda (IoT) de nível industrial baseado em ESP32-CAM, comunicação MQTT segura via mTLS (porta 8883) e backend de mídia em Go.

O sistema resolve o limite físico do ESP32-CAM (capping de 1 conexão HTTP simultânea) através de:
1. **Ingestão Centralizada Única:** 1 única conexão mantida entre o Media Gateway e a câmera.
2. **Broadcaster em Memória:** Distribui o fluxo MJPEG para $N$ clientes web simultâneos sem sobrecarregar a câmera.
3. **Gravação Orientada a Presença Contínua:** Grava em disco via FFmpeg apenas quando há movimento/presença detectada pelo PIR, usando **janela deslizante de 30s** com renovações periódicas a cada 15s enquanto a presença for contínua, consolidando todo o período em um único arquivo MP4 com átomo `moov` preservado.
4. **Autonomia LAN:** 100% autônomo na rede local, sem dependência de nuvem.

---

## 2. Diagrama de Arquitetura

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
       │                [ PostgreSQL :5435 ]
       ▼
 [ Broadcaster em Memória ]
       ├──> Live Web UI (:8080/)
       ├──> N Espectadores Simultâneos (GET /live?device_id=...)
       └──> Recording Controller (Sliding Window 30s) -> Arquivo MP4
```

---

## 3. Invariantes Arquiteturais Críticos

1. **Capping de 1 Conexão no ESP32-CAM:**
   * O firmware da ESP32-CAM rejeita com HTTP 403 qualquer conexão secundária.
   * **NUNCA** faça conexões diretas da UI ou de processos FFmpeg para a câmera física.
   * Toda visualização e gravação DEVE passar pelo `stream.Broadcaster` centralizado (`http://127.0.0.1:8080/live?device_id=...`).
2. **Gravação Orientada a Eventos PIR:**
   * Nenhuma gravação é persistida em disco sem evento de presença válido via MQTT. O fluxo ao vivo fica apenas em memória.
3. **Presença Contínua e Janela Deslizante (Sliding Window de 30s):**
   * Primeiro evento PIR inicia a gravação com tolerância de 30 segundos.
   * Enquanto houver presença física (sensor em nível alto ou novos pulsos), a borda (ESP32-CAM / ESP-PIR) renova o evento MQTT a cada 15s (`eventCooldown` / `INTERVALO_RENOVACAO_MS`).
   * Cada renovação reinicia a janela de 30s no `media-gateway`, garantindo que **a gravação não pare enquanto o PIR seguir percebendo presença**.
   * Quando o ambiente é desocupado, após 10s de calmaria (`motionHoldTime`), a borda entra em repouso. 30s após o último evento, o `media-gateway` encerra a sessão.
   * Não fragmenta o vídeo: todo o evento contínuo gera exatamente **1 único arquivo MP4**.
4. **Finalização Graciosa do FFmpeg:**
   * O encerramento de gravação envia `q` pelo `stdin` do FFmpeg (ou `SIGINT`), aguardando a escrita do átomo `moov` (MP4) para evitar corrupção de arquivo.
5. **Protocolo Binário de Rede:**
   * Magic Number `0x53 0x4E` ("SN"), timestamp `uint64` Big-Endian, tipo `0x01` (Heartbeat) ou `0x02` (Evento PIR).
   * MQTT não transporta frames de vídeo, apenas controle e telemetria.
6. **Autonomia Local:** Operação estritamente local (LAN), sem serviços externos.

---

## 4. Mapa de Módulos do Repositório

```
SentinelNode/
├── certs/                     # Certificados x509 mTLS e gerador Go/Shell
├── docker-compose.yaml        # Mosquitto (1883, 8883 mTLS, 9001) e PostgreSQL 15 (porta 5435)
├── edge-agent/                # Agente de borda simulador em Go (heartbeat e alertas)
├── firmware/
│   ├── src/main.cpp           # Firmware modular C++ ESP32-CAM (FreeRTOS, Watchdog, mTLS, HTTP MJPEG)
│   ├── lib/                   # Drivers (CameraDriver, PirDriver, HttpStreamService, MqttService, etc.)
│   ├── ESP-PIR/               # Firmware dedicado em C para sensor PIR em ESP32 autônomo
│   └── ESP-CAM/emulator.py    # Emulador de câmera em Python (MJPEG :8087, mTLS MQTT, endpoints de presença)
├── ingestion-engine/          # Motor Go que consome MQTT mTLS, persiste no Postgres e aciona o Gateway
└── media-gateway/             # Servidor de vídeo Go (:8080)
    ├── internal/stream/       # Ingestão única, FrameReader e Broadcaster circular
    ├── internal/recorder/     # RecordingController com Sliding Window e MetadataStore (manifest.json)
    └── internal/web/          # Dashboard web HTML5/CSS/JS embutido
```

---

## 5. Protocolo Binário de Rede (Magic `0x53 0x4E`)

### Heartbeat (Tipo `0x01`):
* `[0..1]`: `0x53`, `0x4E`
* `[2]`: `0x01`
* `[3..10]`: Timestamp Unix Epoch (8 bytes Big-Endian)
* `[11]`: Tamanho do Device ID (1 byte)
* `[12..N]`: Device ID string
* `[...]`: Status (1 byte: `0x01` = ativo, `0x02` = inativo)
* `[...]`: Firmware Major, Minor, Patch (3 bytes)
* `[...]`: Tamanho da Stream URL (2 bytes uint16 Big-Endian)
* `[...]`: Stream URL string

### Evento PIR (Tipo `0x02`):
* `[0..1]`: `0x53`, `0x4E`
* `[2]`: `0x02`
* `[3..10]`: Timestamp Unix Epoch (8 bytes Big-Endian)
* `[11]`: Tamanho do Device ID (1 byte)
* `[12..N]`: Device ID string
* `[...]`: Tipo de Evento (1 byte: `0x01` = `motion_detected`, `0x02` = `motion_ended`/outro)
* `[...]`: Tamanho da Video URL (2 bytes uint16 Big-Endian)
* `[...]`: Video URL string (opcional)

---

## 6. Portas e Endpoints

| Serviço | Porta | Descrição |
|:---|:---:|:---|
| **Media Gateway** | `8080` | Web UI (`/`), Live (`/live`), Status (`/status`), Gravação (`/record`), Gravações (`/api/recordings`), MP4 (`/recordings/*.mp4`) |
| **Mosquitto mTLS**| `8883` | Broker MQTT autenticado por certificados x509 cliente/CA |
| **Mosquitto TCP** | `1883` | Broker MQTT texto puro local |
| **Mosquitto WS**  | `9001` | WebSockets MQTT |
| **PostgreSQL 15** | `5435` | Banco de dados (host `localhost:5435`, user `sentinel`, db `sentinel_db`) |
| **Emulador Python**| `8087`| Stream MJPEG (`/stream`), Trigger PIR (`/pir/trigger`), Presença (`/pir/presence/start`, `/stop`, `/status`) |

---

## 7. Como Executar e Testar

### Infraestrutura
```bash
docker compose up -d
```

### Media Gateway
```bash
cd media-gateway && go run main.go
# Interface Web: http://localhost:8080/
```

### Ingestion Engine
```bash
cd ingestion-engine && go run cmd/main.go
```

### Emulador de Câmera (sem hardware físico)
```bash
cd firmware/ESP-CAM && python3 emulator.py
```

### Suíte Completa de Testes
```bash
# 1. Media Gateway (Broadcaster, Recorder e 11 cenários de integração)
cd media-gateway && go test -v ./...

# 2. Ingestion Engine
cd ../ingestion-engine && go test -v ./...

# 3. Edge Agent
cd ../edge-agent && go test -v ./...

# 4. Testes de Protocolo C++
cd ../firmware/test && ./test_protocol

# 5. Build PlatformIO (Firmwares ESP32)
cd ../firmware && pio run
cd ESP-PIR && pio run
```

---

## 8. Mapeamento Flexível 1-para-N (PIR para Múltiplas Câmeras)

O `ingestion-engine` suporta disparar múltiplas câmeras simultâneas a partir de um único sensor PIR.

Configuração via variável de ambiente `PIR_CAMERA_MAPPING`:
```env
# Formato: pir_id:cam1,cam2;pir_id2:cam3,cam4
PIR_CAMERA_MAPPING=esp32-pir-01:esp32-cam-01,esp32-cam-02
```

Quando `esp32-pir-01` detectar presença:
1. O Ingestion Engine localiza todas as câmeras associadas.
2. Puxa os streams ativos de cada câmera no cache.
3. Dispara a gravação independente de cada uma no `media-gateway`.
4. Ambas as câmeras continuam gravando em paralelo enquanto o PIR mantiver detecção de presença.

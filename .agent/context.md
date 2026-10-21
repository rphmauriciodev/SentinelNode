# ESPECIFICAÇÃO TÉCNICA E CONTEXTO DE ENGENHARIA

## 1. Hardware e Firmware de Borda

### ESP32-CAM (AI-Thinker)
* **Microcontrolador:** ESP32 Dual Core Tensilica Xtensa 32-bit LX6 com 520 KB SRAM e 4 MB PSRAM externa.
* **Sensor de Imagem:** OV2640 / OV3660 via interface DVP (8-bit bus com XCLK e PCLK).
* **Sensor de Presença (PIR):** GPIO 13 com tratamento de debounce e filtro temporal.
* **Rede:** Wi-Fi 802.11 b/g/n (2.4 GHz).
* **Stream de Vídeo:** HTTP MJPEG (`multipart/x-mixed-replace;boundary=123456789000000000000987654321`) exposto na porta 80 em `/stream`.
* **Capping Físico de Conexão:** No firmware (`HttpStreamService.cpp`), o servidor HTTP permite apenas 1 cliente simultâneo por restrição de banda e memória na SPIRAM. Conexões adicionais recebem HTTP 403 Forbidden.

### ESP-PIR
* Módulo ESP32 dedicado conectado a um sensor PIR físico no GPIO 27.
* Executa FreeRTOS com pilha LwIP e cliente MQTT mTLS nativo em C.
* Emite pacotes binários de eventos com `event_type = 0x01` (`motion_detected`) para o tópico `sentinel/devices/esp32-pir-01/events`.

---

## 2. Protocolo Binário de Comunicação

Elimina o overhead de JSON na rede IoT de baixa potência. Utiliza Magic Number `0x53 0x4E` ("SN"):

### Heartbeat (Tipo 0x01)
```
[0..1]   : Magic Number (0x53, 0x4E)
[2]      : Tipo de Pacote (0x01)
[3..10]  : Timestamp Unix Epoch (8 bytes Big-Endian)
[11]     : Tamanho do Device ID (1 byte)
[12..N]  : Device ID string
[...]    : Status (1 byte: 0x01 = ativo, 0x02 = inativo)
[...]    : Firmware Major (1 byte)
[...]    : Firmware Minor (1 byte)
[...]    : Firmware Patch (1 byte)
[...]    : Tamanho da Stream URL (2 bytes uint16 Big-Endian)
[...]    : Stream URL string
```

### Evento PIR (Tipo 0x02)
```
[0..1]   : Magic Number (0x53, 0x4E)
[2]      : Tipo de Pacote (0x02)
[3..10]  : Timestamp Unix Epoch (8 bytes Big-Endian)
[11]     : Tamanho do Device ID (1 byte)
[12..N]  : Device ID string
[...]    : Tipo de Evento (1 byte: 0x01 = motion_detected, 0x02 = outro)
[...]    : Tamanho da Video URL (2 bytes uint16 Big-Endian)
[...]    : Video URL string (opcional)
```

---

## 3. Arquitetura do Backend (Go)

### Media Gateway (`media-gateway`)
* **`internal/stream/`**:
  * `FrameReader`: Parser de boundaries HTTP multipart MJPEG.
  * `Broadcaster`: Multiplexador não-bloqueante com fila circular em memória.
  * `CameraStream`: Ingestão contínua com reconexão automática e telemetria (FPS, frames, status).
* **`internal/recorder/`**:
  * `RecordingController`: Gerenciador de ciclo de vida com sliding window de 30s.
  * `MetadataStore`: Armazenamento de metadados com serialização em `manifest.json`.
* **`internal/web/`**:
  * Painel web em HTML5 puro embutido (`go:embed dashboard.html`).

### Ingestion Engine (`ingestion-engine`)
* Conexão mTLS com Mosquitto na porta 8883.
* Subscrição em `sentinel/devices/+/heartbeat` e `sentinel/devices/+/events`.
* Mapeamento estático e dinâmico de sensores PIR para câmeras correspondentes.
* Fila em memória com backpressure para inserção no PostgreSQL com backoff exponencial.

---

## 4. Banco de Dados e Esquema de Eventos

Tabela `security_events` no PostgreSQL:
```sql
CREATE TABLE IF NOT EXISTS security_events (
    id SERIAL PRIMARY KEY,
    device_id VARCHAR(50) NOT NULL,
    event_type VARCHAR(50) NOT NULL,
    video_url TEXT NOT NULL,
    timestamp TIMESTAMP NOT NULL
);
```

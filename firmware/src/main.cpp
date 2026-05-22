// ==============================================================================
// SentinelNode ESP32-CAM - Main Orchestrator (Production Firmware)
// Resilient, modular, non-blocking and protected by hardware Task Watchdog
// ==============================================================================

#include <Arduino.h>
#include <time.h>
#include <esp_system.h>

// Custom system libraries (modular architecture)
#include "WatchdogSystem.h"
#include "CameraDriver.h"
#include "PirDriver.h"
#include "WifiService.h"
#include "MqttService.h"
#include "HttpStreamService.h"

// Firmware constants
const char* ssid = WIFI_SSID;
const char* password = WIFI_PASS;
const char* mqtt_broker = MQTT_BROKER_IP;
const int mqtt_port = 8883;
const char* device_id = "esp32-cam-01";

// Versioning parameters
const uint8_t FIRMWARE_MAJOR = 1;
const uint8_t FIRMWARE_MINOR = 0;
const uint8_t FIRMWARE_PATCH = 0;

// Application states
char video_url[128] = "";
bool motionActive = false;

// Non-blocking timers
unsigned long lastHeartbeat = 0;
unsigned long lastTelemetry = 0;
unsigned long lastEventTime = 0;

// Constants timing
const unsigned long heartbeatInterval = 30000; // 30 segundos
const unsigned long telemetryInterval = 10000; // 10 segundos
const unsigned long eventCooldown     = 15000; // 15 segundos (debounce de envio)
const unsigned long motionHoldTime    = 10000; // 10 segundos (tempo mínimo de trava)

// Forward declarations
void printResetReason();
void syncNTPTime();
void updateVideoUrl();
void logTelemetry();

void setup() {
    // ESTÁGIO 1: Console Serial & Diagnósticos do Boot
    Serial.begin(115200);
    Serial.flush(); // Garante envio dos buffers pendentes
    delay(300);     // Dá tempo para o CH340 de hardware se sincronizar com a USB do host
    
    Serial.println("\n\n======================================================================");
    Serial.printf("SENTINELNODE FIRMWARE v%d.%d.%d (PRODUÇÃO - ESP32 Arduino Core 2.x)\n", 
                  FIRMWARE_MAJOR, FIRMWARE_MINOR, FIRMWARE_PATCH);
    Serial.println("======================================================================");

    // Decodifica a causa do último reinício (útil para diagnosticar crashes/brownouts)
    printResetReason();
    
    // ESTÁGIO 2: Hardware Watchdog System (Timeout de 15s para suportar handshakes de criptografia)
    WatchdogSystem::init(15);
    WatchdogSystem::feed();

    // ESTÁGIO 3: Sensores Físicos (PIR)
    // Inicializa o PIR com tratamento robusto de glitch temporal
    PirDriver::init(13);
    WatchdogSystem::feed();

    // ESTÁGIO 4: Conectividade Wi-Fi e Pilha de Rede (LwIP)
    // Inicializar o Wi-Fi primeiro garante que a pilha TCP/IP (LwIP) esteja ativa antes de qualquer soquete
    Serial.println("[SYSTEM] Inicializando conexão de rede Wi-Fi e pilha TCP/IP (LwIP)...");
    WifiService::init(ssid, password);
    WatchdogSystem::feed();

    // ESTÁGIO 5: Inicialização da Câmera (Reset físico + Fallback dinâmico)
    Serial.println("[SYSTEM] Iniciando subsistema da Câmera...");
    bool cameraOk = CameraDriver::init();
    WatchdogSystem::feed();
    delay(100); // Pequena pausa para acomodar picos transientes pós boot da câmera

    // Se o Wi-Fi conseguiu se conectar no boot, sincroniza relógio (NTP)
    if (WifiService::isConnected()) {
        syncNTPTime();
        updateVideoUrl();
    }
    WatchdogSystem::feed();

    // ESTÁGIO 6: Inicialização de Servidores
    if (cameraOk) {
        // Inicializa o servidor HTTP para streaming de vídeo (agora com LwIP ativo e seguro)
        HttpStreamService::start();
    } else {
        Serial.println("[SYSTEM WARNING] Câmera falhou ao subir. O sistema operará somente com sensores.");
    }
    WatchdogSystem::feed();

    // ESTÁGIO 7: Barramento Seguro TLS MQTT (mTLS)
    Serial.println("[SYSTEM] Inicializando barramento de eventos seguro TLS MQTT...");
    MqttService::init(device_id, mqtt_broker, mqtt_port);
    
    // Alimenta o Watchdog ao concluir a inicialização em estágios
    WatchdogSystem::feed();
    Serial.println("[SYSTEM] Setup concluído! SentinelNode está ativo e monitorando...");
}

void loop() {
    // 1. Alimenta o hardware Watchdog a cada iteração do loop principal
    WatchdogSystem::feed();

    // 2. Executa manutenção periódica de conexões (não-bloqueante)
    WifiService::handle();
    MqttService::handle();

    unsigned long now = millis();

    // 3. Gerencia a transição de reconexão do Wi-Fi para disparar eventos
    if (WifiService::checkJustConnected()) {
        Serial.println("[SYSTEM] Wi-Fi reconectado. Sincronizando relógio e endpoint...");
        syncNTPTime();
        updateVideoUrl();
        WatchdogSystem::feed();
    }

    // 4. Envio de Heartbeat binário periódico a cada 30 segundos (via MQTT seguro)
    if (MqttService::isConnected() && (now - lastHeartbeat >= heartbeatInterval)) {
        lastHeartbeat = now;
        MqttService::sendHeartbeat(0x01, FIRMWARE_MAJOR, FIRMWARE_MINOR, FIRMWARE_PATCH, video_url);
    }

    // 5. Processamento de Eventos PIR (Gatilho rápido e assíncrono via interrupção validada)
    if (PirDriver::hasTriggered()) {
        // Verifica se a trava de cooldown expirou antes de enviar nova mensagem
        if (!motionActive && (now - lastEventTime >= eventCooldown)) {
            Serial.println("[SYSTEM EVENT] Movimento validado sustentado pelo sensor PIR!");
            motionActive = true;
            lastEventTime = now;
            
            if (MqttService::isConnected()) {
                MqttService::sendEvent(0x01, video_url); // Tipo de evento: 0x01 (motion)
            } else {
                Serial.println("[SYSTEM WARNING] Alerta gerado, mas descartado: MQTT está offline.");
            }
        }
    }

    // 6. Normalização automática do estado de alerta (tempo mínimo de amostragem lógico)
    if (motionActive && (now - lastEventTime >= motionHoldTime)) {
        // Se o sensor físico voltou a repousar, limpa o estado de alerta
        if (!PirDriver::readStatus()) {
            Serial.println("[SYSTEM EVENT] Área normalizada. Sensor PIR em repouso.");
            motionActive = false;
        }
    }

    // 7. Escrita de logs de telemetria no console serial a cada 10 segundos
    if (now - lastTelemetry >= telemetryInterval) {
        lastTelemetry = now;
        logTelemetry();
    }

    // Cede 10ms para que o kernel do FreeRTOS cuide de tarefas secundárias da CPU (ex: Wi-Fi/TLS)
    vTaskDelay(pdMS_TO_TICKS(10));
}

void printResetReason() {
    esp_reset_reason_t reason = esp_reset_reason();
    Serial.print("[BOOT DIAG] Causa do último reset: ");
    switch (reason) {
        case ESP_RST_POWERON:   Serial.println("POWER-ON (Energização normal)"); break;
        case ESP_RST_SW:        Serial.println("SOFTWARE RESET (Solicitado por código via esp_restart)"); break;
        case ESP_RST_PANIC:     Serial.println("EXCEPTION PANIC (Crash/Falha crítica de software)"); break;
        case ESP_RST_INT_WDT:   Serial.println("INTERRUPT WATCHDOG (CPU travada)"); break;
        case ESP_RST_TASK_WDT:  Serial.println("TASK WATCHDOG (Task principal estourou o timeout)"); break;
        case ESP_RST_WDT:       Serial.println("OTHER WATCHDOGS (Watchdog de hardware secundário)"); break;
        case ESP_RST_DEEPSLEEP: Serial.println("DEEP SLEEP EXIT (Acordou de hibernação)"); break;
        case ESP_RST_BROWNOUT:  Serial.println("BROWNOUT DETECTED (Queda abrupta de tensão na alimentação!)"); break;
        case ESP_RST_SDIO:      Serial.println("SDIO RESET (Reset via SDIO)"); break;
        default:                Serial.println("INDETERMINADO"); break;
    }
}

void syncNTPTime() {
    Serial.println("[NTP] Sincronizando relógio via NTP...");
    configTime(0, 0, "pool.ntp.org", "b.ntp.br");

    time_t now = time(nullptr);
    int attempts = 0;
    
    // Aguarda sincronização por no máximo 15 segundos
    while (now < 24 * 3600 && attempts < 30) {
        // CRÍTICO: alimenta o watchdog enquanto bloqueia aguardando pacotes NTP na rede!
        WatchdogSystem::feed();
        delay(500);
        Serial.print(".");
        now = time(nullptr);
        attempts++;
    }
    
    if (now >= 24 * 3600) {
        Serial.printf("\n[NTP] Relógio sincronizado com sucesso! Unix Epoch: %ld\n", now);
    } else {
        Serial.println("\n[NTP WARNING] Tempo limite esgotado para o NTP. Usando relógio interno aproximado.");
    }
}

void updateVideoUrl() {
    String ipStr = WifiService::getIP();
    snprintf(video_url, sizeof(video_url), "http://%s/stream", ipStr.c_str());
    Serial.printf("[SYSTEM] Endpoint de vídeo atualizado: %s\n", video_url);
}

void logTelemetry() {
    Serial.printf("[TELEMETRY] Heap Livre: %u B | Bloco Contíguo Máx: %u B | Mínimo Histórico: %u B | PSRAM Livre: %u B | PIR: %s | Streaming: %s\n",
                  ESP.getFreeHeap(),
                  ESP.getMaxAllocHeap(), // Diagnóstico primordial contra heap fragmentation
                  ESP.getMinFreeHeap(),
                  ESP.getFreePsram(),
                  PirDriver::readStatus() ? "DETECTADO" : "LIMPO",
                  HttpStreamService::isStreaming() ? "EM EXECUÇÃO" : "INATIVO");
}

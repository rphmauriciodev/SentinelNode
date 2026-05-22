// ==============================================================================
// SentinelNode ESP32-CAM - Main Orchestrator (Production Firmware)
// Resilient, modular, non-blocking and protected by hardware Task Watchdog
// ==============================================================================

#include <Arduino.h>
#include <time.h>

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
void syncNTPTime();
void updateVideoUrl();
void logTelemetry();

void setup() {
    // 1. Inicializa console serial para diagnósticos
    Serial.begin(115200);
    delay(500);
    Serial.println("\n\n======================================================================");
    Serial.printf("SENTINELNODE FIRMWARE v%d.%d.%d (ESTÁVEL - ESP32 Arduino Core 2.0.x)\n", 
                  FIRMWARE_MAJOR, FIRMWARE_MINOR, FIRMWARE_PATCH);
    Serial.println("======================================================================");

    // 2. Inicializa o hardware Watchdog com timeout seguro de 12 segundos
    WatchdogSystem::init(12);

    // 3. Inicializa o sensor de movimento PIR (GPIO 13) usando interrupções físicas
    PirDriver::init(13);

    // 4. Inicializa o serviço Wi-Fi (máquina de estados híbrida)
    WifiService::init(ssid, password);
    WatchdogSystem::feed();

    // 5. Se o Wi-Fi conseguiu se conectar no boot, sincroniza o relógio (NTP)
    if (WifiService::isConnected()) {
        syncNTPTime();
        updateVideoUrl();
    }
    WatchdogSystem::feed();

    // 6. Inicializa o barramento de comunicação segura TLS MQTT (mTLS)
    MqttService::init(device_id, mqtt_broker, mqtt_port);
    WatchdogSystem::feed();

    // 7. Inicializa o sensor físico da câmera (OV3660 calibrado a 15MHz)
    if (CameraDriver::init()) {
        // Se a câmera subiu, inicia o servidor HTTP para streaming de vídeo
        HttpStreamService::start();
    } else {
        Serial.println("[SYSTEM WARNING] Câmera falhou ao subir. O sistema rodará apenas como sensor PIR!");
    }
    
    // Alimenta o Watchdog ao concluir a inicialização
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
        Serial.println("[SYSTEM] Wi-Fi reconectado. Atualizando parâmetros de rede...");
        syncNTPTime();
        updateVideoUrl();
        WatchdogSystem::feed();
    }

    // 4. Envio de Heartbeat binário periódico a cada 30 segundos (via MQTT seguro)
    if (MqttService::isConnected() && (now - lastHeartbeat >= heartbeatInterval)) {
        lastHeartbeat = now;
        MqttService::sendHeartbeat(0x01, FIRMWARE_MAJOR, FIRMWARE_MINOR, FIRMWARE_PATCH, video_url);
    }

    // 5. Processamento de Eventos PIR (Gatilho rápido e assíncrono via interrupção)
    if (PirDriver::hasTriggered()) {
        // Verifica se a trava de cooldown expirou antes de enviar nova mensagem
        if (!motionActive && (now - lastEventTime >= eventCooldown)) {
            Serial.println("[SYSTEM EVENT] Movimento capturado pelo sensor PIR!");
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
    Serial.printf("[TELEMETRY] Free Heap: %u B | Min Free Heap: %u B | Free PSRAM: %u B | PIR Sensor: %s | Video Stream: %s\n",
                  ESP.getFreeHeap(),
                  ESP.getMinFreeHeap(),
                  ESP.getFreePsram(),
                  PirDriver::readStatus() ? "DETECTADO" : "LIMPO",
                  HttpStreamService::isStreaming() ? "EM EXECUÇÃO" : "INATIVO");
}

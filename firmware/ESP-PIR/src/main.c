#include <Arduino.h>
#include <time.h>
#include <string.h>
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
#include "esp_wifi.h"
#include "esp_event.h"
#include "nvs_flash.h"
#include "mqtt_client.h"
#include "esp_netif.h"
#include "mdns.h"
#include "esp_sntp.h"
#include "esp_system.h"

#include "../../include/secrets.h"
#include "../../include/protocol.h"

#define SENSOR_PIR       27               // GPIO 27 para o PIR
#define LIMITE_TEMPO_MS  10000            // Tempo de calmaria (10 segundos)
#define DEVICE_ID        "esp32-pir-01"  // ID único deste dispositivo

// Timers não-bloqueantes
static uint32_t last_heartbeat = 0;
static uint32_t last_telemetry = 0;
static uint32_t tempo_anterior_ms = 0;
static bool cronometro_rodando = false;

static esp_mqtt_client_handle_t client = NULL;
static bool wifi_connected = false;
static bool mqtt_connected = false;
static bool motion_active = false;

// Declarações externas das funções pontes C++ do bridge.cpp
extern void conectar_wifi_cpp(const char* ssid, const char* password);
extern bool wifi_conectado_cpp(void);
extern void obter_ip_cpp(char* ip_str, size_t max_len);
extern int obter_status_wifi_cpp(void);

void inicializar_wifi(void) {
    printf("[WIFI] Inicializando conexão de rede...\n");
    printf("[WIFI] SSID: %s\n", WIFI_SSID);
    conectar_wifi_cpp(WIFI_SSID, WIFI_PASS);
}

// Inicialização do cliente de resolução de nomes local mDNS
void inicializar_mdns(void) {
    esp_err_t err = mdns_init();
    if (err) {
        printf("[mDNS] Falha ao inicializar mDNS: %d\n", err);
    } else {
        printf("[mDNS] Inicializado com sucesso.\n");
    }
}

// Handler de eventos do cliente MQTT
static void mqtt_event_handler(void *handler_args, esp_event_base_t base, int32_t event_id, void *event_data) {
    esp_mqtt_event_handle_t event = event_data;
    switch ((esp_mqtt_event_id_t)event_id) {
        case MQTT_EVENT_CONNECTED:
            mqtt_connected = true;
            printf("[MQTT] Conectado ao Broker TLS com sucesso!\n");
            break;
        case MQTT_EVENT_DISCONNECTED:
            mqtt_connected = false;
            printf("[MQTT] Desconectado do Broker TLS.\n");
            break;
        default:
            break;
    }
}

// Inicialização do cliente MQTT mTLS
void inicializar_mqtt(void) {
    printf("[MQTT] Inicializando cliente seguro TLS (mTLS)...\n");

    char uri[128];
    snprintf(uri, sizeof(uri), "mqtts://%s:8883", MQTT_BROKER_IP);
    printf("[MQTT] URI configurada: %s\n", uri);

    // Estrutura de configuração flat (padrão do ESP-IDF v4.4 embutido no Arduino Core 2.x)
    esp_mqtt_client_config_t mqtt_cfg = {
        .uri = uri,
        .cert_pem = ca_cert,
        .client_cert_pem = client_cert,
        .client_key_pem = client_key,
        .skip_cert_common_name_check = true,
    };

    client = esp_mqtt_client_init(&mqtt_cfg);
    if (client == NULL) {
        printf("[MQTT ERROR] Falha ao inicializar instância do cliente MQTT.\n");
        return;
    }

    esp_mqtt_client_register_event(client, ESP_EVENT_ANY_ID, mqtt_event_handler, NULL);
    esp_mqtt_client_start(client);
}

// Sincroniza o relógio interno por NTP para timestamping preciso dos pacotes binários
void sincronizar_tempo_ntp(void) {
    printf("[NTP] Sincronizando relógio via NTP...\n");
    sntp_setoperatingmode(SNTP_OPMODE_POLL);
    sntp_setservername(0, "pool.ntp.org");
    sntp_setservername(1, "b.ntp.br");
    sntp_init();

    time_t now = time(NULL);
    int attempts = 0;
    while (now < 24 * 3600 && attempts < 30) {
        vTaskDelay(500 / portTICK_PERIOD_MS);
        printf(".");
        now = time(NULL);
        attempts++;
    }

    if (now >= 24 * 3600) {
        printf("\n[NTP] Relógio sincronizado com sucesso! Unix Epoch: %ld\n", (long)now);
    } else {
        printf("\n[NTP WARNING] Tempo limite esgotado para o NTP. Usando relógio interno aproximado.\n");
    }
}

bool enviar_heartbeat(void) {
    if (!mqtt_connected || client == NULL) {
        return false;
    }

    uint8_t buf[256];
    time_t now = time(NULL);

    int size = encodeHeartbeat(buf, DEVICE_ID, (uint64_t)now, 1, 1, 0, 0, "");

    char topic[128];
    snprintf(topic, sizeof(topic), "sentinel/devices/%s/heartbeat", DEVICE_ID);

    int msg_id = esp_mqtt_client_publish(client, topic, (const char *)buf, size, 1, 0);
    if (msg_id >= 0) {
        printf("[MQTT] Pacote binário de Heartbeat publicado (%d bytes) no tópico: %s\n", size, topic);
        return true;
    } else {
        printf("[MQTT ERROR] Falha ao publicar Heartbeat!\n");
        return false;
    }
}

bool enviar_evento(uint8_t event_type) {
    if (!mqtt_connected || client == NULL) {
        return false;
    }

    uint8_t buf[256];
    time_t now = time(NULL);

    // Event type: 1 = motion_detected, 2 = motion_ended (limpo)
    int size = encodeEvent(buf, DEVICE_ID, (uint64_t)now, event_type, "");

    char topic[128];
    snprintf(topic, sizeof(topic), "sentinel/devices/%s/events", DEVICE_ID);

    int msg_id = esp_mqtt_client_publish(client, topic, (const char *)buf, size, 1, 0);
    if (msg_id >= 0) {
        printf("[MQTT] Evento binário publicado (%d bytes, tipo: %d) no tópico: %s\n", size, event_type, topic);
        return true;
    } else {
        printf("[MQTT ERROR] Falha ao publicar Evento!\n");
        return false;
    }
}

void setup_c() {
    delay(500);
    printf("\n\n======================================================================\n");
    printf("SENTINELNODE ESP-PIR FIRMWARE (C NATIVO - mTLS MQTT)\n");
    printf("======================================================================\n");

    esp_err_t ret = nvs_flash_init();
    if (ret == ESP_ERR_NVS_NO_FREE_PAGES || ret == ESP_ERR_NVS_NEW_VERSION_FOUND) {
        nvs_flash_erase();
        nvs_flash_init();
    }

    pinMode(SENSOR_PIR, INPUT_PULLDOWN);
    printf("[PIR] Sensor físico configurado no pino GPIO %d com PULLDOWN ativo.\n", SENSOR_PIR);

    // Inicializa Wi-Fi via ponte
    inicializar_wifi();

    // Bloqueia no boot até conectar no Wi-Fi por um limite seguro
    printf("[WIFI] Aguardando conexão Wi-Fi...");
    int wifi_attempts = 0;
    while (!wifi_conectado_cpp() && wifi_attempts < 40) {
        vTaskDelay(500 / portTICK_PERIOD_MS);
        printf(".");
        wifi_attempts++;
    }
    printf("\n");

    if (wifi_conectado_cpp()) {
        wifi_connected = true;
        char ip_str[32];
        obter_ip_cpp(ip_str, sizeof(ip_str));
        printf("[WIFI] Conectado com sucesso! IP obtido: %s\n", ip_str);

        inicializar_mdns();
        sincronizar_tempo_ntp();
        inicializar_mqtt();
    } else {
        printf("[WIFI WARNING] Falha ao se conectar no Wi-Fi durante o boot. Código de status: %d\n", obter_status_wifi_cpp());
    }
}

void loop_c() {
    uint32_t now = millis();

    bool current_wifi_connected = wifi_conectado_cpp();
    if (current_wifi_connected != wifi_connected) {
        wifi_connected = current_wifi_connected;
        if (wifi_connected) {
            char ip_str[32];
            obter_ip_cpp(ip_str, sizeof(ip_str));
            printf("[WIFI] Conectado! IP obtido: %s\n", ip_str);
        } else {
            printf("[WIFI] Conexão Wi-Fi perdida!\n");
            mqtt_connected = false;
        }
    }
    bool movimento_detectado = (digitalRead(SENSOR_PIR) == HIGH);

    if (movimento_detectado) {
        if (!motion_active) {
            motion_active = true;
            printf("[MOVIMENTO] Detectado! Enviando sinal MQTT...\n");
            enviar_evento(1); 
        }
        tempo_anterior_ms = now;
        cronometro_rodando = true;
    }

    if (cronometro_rodando) {
        if (now - tempo_anterior_ms >= LIMITE_TEMPO_MS) {
            if (!movimento_detectado) {
                motion_active = false;
                cronometro_rodando = false;
                printf("[TIMER] %d s sem movimento. Enviando fim de alerta MQTT...\n", LIMITE_TEMPO_MS / 1000);
                enviar_evento(2); // 2 = motion_ended (repouso)
            } else {
                tempo_anterior_ms = now;
            }
        }
    }

    if (mqtt_connected && (now - last_heartbeat >= 30000)) {
        last_heartbeat = now;
        enviar_heartbeat();
    }

    if (now - last_telemetry >= 10000) {
        last_telemetry = now;
        printf("[TELEMETRY] Heap Livre: %u B | PIR: %s | WiFi: %s | MQTT: %s\n",
               (unsigned int)esp_get_free_heap_size(),
               movimento_detectado ? "DETECTADO" : "LIMPO",
               wifi_connected ? "CONECTADO" : "DESCONECTADO",
               mqtt_connected ? "CONECTADO" : "DESCONECTADO");
    }

    vTaskDelay(50 / portTICK_PERIOD_MS);
}

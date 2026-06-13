#include <stdio.h>
#include "driver/gpio.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
#include "esp_wifi.h"
#include "esp_event.h"
#include "nvs_flash.h"
#include "mqtt_client.h"
#include "WiFi.h" 

#define SENSOR_PIR       GPIO_NUM_27     // Pino do Sensor de Presença
#define LIMITE_TEMPO_MS  5000            // Tempo de calmaria (5 segundos)
#define MQTT_BROKER_URI  "mqtt://://" MQTT_BROKER_IP ":1883"
#define MQTT_TOPICO      "casa/sensor/presenca"

typedef struct {
  bool movimento_detectado;
  bool alerta_ativo;
} EstadoSistema;

static EstadoSistema sistema;
static esp_mqtt_client_handle_handle_t cliente_mqtt = NULL;

static uint32_t tempo_anterior_ms = 0;
static bool cronometro_rodando = false;
static bool mqtt_conectado = false;

static void mqtt_event_handler(void *handler_args, esp_event_base_t base, int32_t event_id, void *event_data) {
  esp_mqtt_event_handle_t event = event_data;
  switch ((esp_mqtt_event_id_t)event_id) {
    case MQTT_EVENT_CONNECTED:
      mqtt_conectado = true;
      printf("[MQTT] Conectado ao Broker com sucesso!\n");
      break;
    case MQTT_EVENT_DISCONNECTED:
      mqtt_conectado = false;
      printf("[MQTT] Desconectado do Broker.\n");
      break;
    default:
      break;
  }
}


void inicializar_mqtt(void) {
  esp_mqtt_client_config_t mqtt_cfg = {
    .broker.address.uri = MQTT_BROKER_URI,
  };
  
  cliente_mqtt = esp_mqtt_client_init(&mqtt_cfg);
  esp_mqtt_client_register_event(cliente_mqtt, ESP_EVENT_ANY_ID, mqtt_event_handler, NULL);
  esp_mqtt_client_start(cliente_mqtt);
}

void inicializar_hardware(void) {
  gpio_reset_pin(SENSOR_PIR);
  gpio_set_direction(SENSOR_PIR, GPIO_MODE_INPUT);
  gpio_set_pull_mode(SENSOR_PIR, GPIO_PULLDOWN_ONLY); 
}

void processar_presenca(EstadoSistema *estado) {
  uint32_t tempo_atual_ms = xTaskGetTickCount() * portTICK_PERIOD_MS;

  estado->movimento_detectado = gpio_get_level(SENSOR_PIR);

  if (estado->movimento_detectado) {
    if (!estado->alerta_ativo) {
      estado->alerta_ativo = true;
      printf("[MOVIMENTO] Detectado! Enviando sinal MQTT...\n");
      
      if (mqtt_conectado && cliente_mqtt != NULL) {
        esp_mqtt_client_publish(cliente_mqtt, MQTT_TOPICO, "1", 0, 1, 0); // Payload "1" = Movimento
      }
    }
    tempo_anterior_ms = tempo_atual_ms;   
    cronometro_rodando = true;
  }

  if (cronometro_rodando) {
    if (tempo_atual_ms - tempo_anterior_ms >= LIMITE_TEMPO_MS) {
      estado->alerta_ativo = false;
      cronometro_rodando = false;
      printf("[TIMER] 5s sem movimento. Enviando sinal de fim de alerta MQTT...\n");
      
      if (mqtt_conectado && cliente_mqtt != NULL) {
        esp_mqtt_client_publish(cliente_mqtt, MQTT_TOPICO, "0", 0, 1, 0); 
      }
    }
  }
}

void setup() {
  printf("Inicializando sistema MQTT em C nativo...\n");
  esp_err_t ret = nvs_flash_init();
  if (ret == ESP_ERR_NVS_NO_FREE_PAGES || ret == ESP_ERR_NVS_NEW_VERSION_FOUND) {
    nvs_flash_erase();
    nvs_flash_init();
  }

  inicializar_hardware();

  WiFi.begin(WIFI_SSID, WIFI_PASS); 
  printf("Conectando ao Wi-Fi...");
  while (WiFi.status() != WL_CONNECTED) {
    vTaskDelay(500 / portTICK_PERIOD_MS);
    printf(".");
  }
  printf("\nWi-Fi Conectado!\n");

  inicializar_mqtt();
}

void loop() {
  processar_presenca(&sistema);
  vTaskDelay(50 / portTICK_PERIOD_MS); 
}

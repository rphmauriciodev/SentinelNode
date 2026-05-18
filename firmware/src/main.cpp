#include <Arduino.h>
#include <WiFi.h>
#include <WiFiClientSecure.h>
#include <PubSubClient.h>
#include <time.h>
#include "esp_camera.h"
#include "esp_http_server.h"

#define Y2_GPIO_NUM        5 
#define Y3_GPIO_NUM       18 
#define Y4_GPIO_NUM       19 
#define Y5_GPIO_NUM       21 
#define Y6_GPIO_NUM       36 
#define Y7_GPIO_NUM       39 
#define Y8_GPIO_NUM       34 
#define Y9_GPIO_NUM       35 
#define VSYNC_GPIO_NUM    25
#define HREF_GPIO_NUM     23 
#define PCLK_GPIO_NUM     22 
#define SIOD_GPIO_NUM     26 
#define SIOC_GPIO_NUM     27 
#define PWDN_GPIO_NUM     32 
#define RESET_GPIO_NUM    -1 
#define PIR_PIN           13

const char* ssid = WIFI_SSID;
const char* password = WIFI_PASS;

const char* mqtt_broker = MQTT_BROKER_IP;
const int mqtt_port = 8883;
const char* device_id = "esp32-cam-01";

const char* ca_cert = R"EOF(
-----BEGIN CERTIFICATE-----
COLE O CONTEÚDO DO ca.crt AQUI
-----END CERTIFICATE-----
)EOF";

const char* client_cert = R"EOF(
-----BEGIN CERTIFICATE-----
COLE O CONTEÚDO DO client.crt AQUI
-----END CERTIFICATE-----
)EOF";

const char* client_key = R"EOF(
-----BEGIN PRIVATE KEY-----
COLE O CONTEÚDO DO client.key AQUI
-----END PRIVATE KEY-----
)EOF";

WiFiClientSecure secureClient;
PubSubClient mqttClient(secureClient);
httpd_handle_t camera_httpd = NULL;

unsigned long lastHeartbeat = 0;
bool motionDetected = false;
String video_url = "";

void connectToWiFi();
void connectToMQTT();
void syncNTPTime();
void startCameraServer();
void sendHeartbeat();
void sendEvent();

void sendHeartbeat() {
    uint8_t buf[128];
    int offset = 0;

    buf[offset++] = 0x53; // Magic1 ('S')
    buf[offset++] = 0x4E; // Magic2 ('N')
    buf[offset++] = 0x01; // Tipo do Pacote: Heartbeat (0x01)

    time_t now = time(nullptr);
    uint64_t ts = (uint64_t)now;
    for (int i = 7; i >= 0; i--) {
        buf[offset++] = (ts >> (i * 8)) & 0xFF;
    }

    uint8_t dev_len = strlen(device_id);
    buf[offset++] = dev_len;
    memcpy(&buf[offset], device_id, dev_len);
    offset += dev_len;
    
    buf[offset++] = 0x01;

    buf[offset++] = 1; // Major
    buf[offset++] = 0; // Minor
    buf[offset++] = 0; // Patch
    String topic = String("sentinel/devices/") + device_id + "/heartbeat";
    mqttClient.publish(topic.c_str(), buf, offset);
    Serial.printf("[MQTT] Heartbeat binário enviado (%d bytes)\n", offset);
}

void sendEvent() {
    uint8_t buf[256];
    int offset = 0;

    buf[offset++] = 0x53; // Magic1 ('S')
    buf[offset++] = 0x4E; // Magic2 ('N')
    buf[offset++] = 0x02; // Tipo do Pacote: Alerta de Evento (0x02)
    time_t now = time(nullptr);
    uint64_t ts = (uint64_t)now;
    for (int i = 7; i >= 0; i--) {
        buf[offset++] = (ts >> (i * 8)) & 0xFF;
    }

    uint8_t dev_len = strlen(device_id);
    buf[offset++] = dev_len;
    memcpy(&buf[offset], device_id, dev_len);
    offset += dev_len;
    buf[offset++] = 0x01;
    uint16_t url_len = video_url.length();
    buf[offset++] = (url_len >> 8) & 0xFF;
    buf[offset++] = url_len & 0xFF;
    memcpy(&buf[offset], video_url.c_str(), url_len);
    offset += url_len;
    String topic = String("sentinel/devices/") + device_id + "/events";
    mqttClient.publish(topic.c_str(), buf, offset);
    Serial.printf("[MQTT] Alerta de movimento binário enviado (%d bytes). URL: %s\n", offset, video_url.c_str());
}

void setup() {
    Serial.begin(115200);
    
    pinMode(PIR_PIN, INPUT);

    camera_config_t config;
    config.ledc_channel = LEDC_CHANNEL_0;
    config.ledc_timer = LEDC_TIMER_0;
    config.pin_d0 = Y2_GPIO_NUM;
    config.pin_d1 = Y3_GPIO_NUM;
    config.pin_d2 = Y4_GPIO_NUM;
    config.pin_d3 = Y5_GPIO_NUM;
    config.pin_d4 = Y6_GPIO_NUM;
    config.pin_d5 = Y7_GPIO_NUM;
    config.pin_d6 = Y8_GPIO_NUM;
    config.pin_d7 = Y9_GPIO_NUM;
    config.pin_xclk = XCLK_GPIO_NUM;
    config.pin_pclk = PCLK_GPIO_NUM;
    config.pin_vsync = VSYNC_GPIO_NUM;
    config.pin_href = HREF_GPIO_NUM;
    config.pin_sscb_sda = SIOD_GPIO_NUM;
    config.pin_sscb_scl = SIOC_GPIO_NUM;
    config.pin_pwdn = PWDN_GPIO_NUM;
    config.pin_reset = RESET_GPIO_NUM;
    config.xclk_freq_hz = 20000000;
    config.pixel_format = PIXFORMAT_JPEG; 

    if (psramFound()) {
        config.frame_size = FRAMESIZE_VGA;
        config.jpeg_quality = 12;
        config.fb_count = 2;
    } else {
        config.frame_size = FRAMESIZE_CIF;
        config.jpeg_quality = 12;
        config.fb_count = 1;
    }

    esp_err_t err = esp_camera_init(&config);
    if (err != ESP_OK) {
        Serial.printf("[CAMERA ERROR] Falha ao inicializar: 0x%x\n", err);
        return;
    }

    secureClient.setCACert(ca_cert);
    secureClient.setCertificate(client_cert);
    secureClient.setPrivateKey(client_key);

    connectToWiFi();
    syncNTPTime();
    video_url = "http://" + WiFi.localIP().toString() + "/stream";
    
    startCameraServer();

    mqttClient.setServer(mqtt_broker, mqtt_port);
    connectToMQTT();
}

void loop() {
    if (WiFi.status() != WL_CONNECTED) {
        connectToWiFi();
    }

    if (!mqttClient.connected()) {
        connectToMQTT();
    }

    mqttClient.loop();
    unsigned long now = millis();
    if (now - lastHeartbeat >= 30000) {
        lastHeartbeat = now;
        sendHeartbeat();
    }

    int pirValue = digitalRead(PIR_PIN);
    
    if (pirValue == HIGH && !motionDetected) {
        Serial.println("[PIR] Alerta! Presença física detectada!");
        motionDetected = true;
        sendEvent();
    } 
    else if (pirValue == LOW && motionDetected) {
        Serial.println("[PIR] Área normalizada. Monitoramento ativo...");
        motionDetected = false;
    }

    delay(10);
}

#define PART_BOUNDARY "123456789000000000000987654321"
static const char* _stream_contentType = "multipart/x-mixed-replace;boundary=" PART_BOUNDARY;
static const char* _stream_boundary = "\r\n--" PART_BOUNDARY "\r\n";
static const char* _stream_part = "Content-Type: image/jpeg\r\nContent-Length: %u\r\n\r\n";

esp_err_t stream_handler(httpd_req_t *req) {
    camera_fb_t * fb = NULL;
    esp_err_t res = ESP_OK;
    size_t _jpg_buf_len = 0;
    uint8_t * _jpg_buf = NULL;
    char * part_buf[64];

    res = httpd_resp_set_type(req, _stream_contentType);
    if (res != ESP_OK) {
        return res;
    }

    while (true) {
        fb = esp_camera_fb_get();
        if (!fb) {
            Serial.println("[CAMERA] Falha ao capturar frame");
            res = ESP_FAIL;
        } else {
            _jpg_buf_len = fb->len;
            _jpg_buf = fb->buf;
        }

        if (res == ESP_OK) {
            res = httpd_resp_send_chunk(req, _stream_boundary, strlen(_stream_boundary));
        }
        if (res == ESP_OK) {
            size_t hlen = snprintf((char *)part_buf, 64, _stream_part, _jpg_buf_len);
            res = httpd_resp_send_chunk(req, (const char *)part_buf, hlen);
        }
        if (res == ESP_OK) {
            res = httpd_resp_send_chunk(req, (const char *)_jpg_buf, _jpg_buf_len);
        }
        if (fb) {
            esp_camera_fb_return(fb);
            fb = NULL;
            _jpg_buf = NULL;
        } else if (_jpg_buf) {
            free(_jpg_buf);
            _jpg_buf = NULL;
        }

        if (res != ESP_OK) {
            break;
        }
    }
    return res;
}

void startCameraServer() {
    httpd_config_t config = HTTPD_DEFAULT_CONFIG();
    config.server_port = 80;

    httpd_uri_t stream_uri = {
        .uri       = "/stream",
        .method    = HTTP_GET,
        .handler   = stream_handler,
        .user_ctx  = NULL
    };

    if (httpd_start(&camera_httpd, &config) == ESP_OK) {
        httpd_register_uri_handler(camera_httpd, &stream_uri);
        Serial.printf("[HTTP] Servidor de câmera ativo em: http://%s/stream\n", WiFi.localIP().toString().c_str());
    }
}

void connectToWiFi() {
    Serial.printf("[WiFi] Conectando a %s...\n", ssid);
    WiFi.begin(ssid, password);
    
    while (WiFi.status() != WL_CONNECTED) {
        delay(500);
        Serial.print(".");
    }
    Serial.println("\n[WiFi] Conectado com sucesso!");
}

void syncNTPTime() {
    Serial.println("[NTP] Sincronizando relógio do chip via NTP...");
    configTime(0, 0, "pool.ntp.org", "b.ntp.br");
    
   time_t now = time(nullptr);
    while (now < 24 * 3600) {
        delay(500);
        Serial.print(".");
        now = time(nullptr);
    }
    Serial.printf("\n[NTP] Relógio sincronizado! Timestamp Unix atual: %ld\n", now);
}

void connectToMQTT() {
    while (!mqttClient.connected()) {
        Serial.printf("[MQTT] Tentando conectar na porta segura 8883 (mTLS)...\n");
        
        if (mqttClient.connect(device_id)) {
            Serial.println("[MQTT] Conectado ao Broker Mosquitto com mTLS!");
        } else {
            Serial.printf("[MQTT ERROR] Falha de conexão. Código de erro: %d. Tentando em 5 segundos...\n", mqttClient.state());
            delay(5000);
        }
    }
}

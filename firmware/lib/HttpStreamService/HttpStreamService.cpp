#include "HttpStreamService.h"
#include "CameraDriver.h"

#define PART_BOUNDARY "123456789000000000000987654321"

httpd_handle_t HttpStreamService::_serverHandle = nullptr;
volatile bool HttpStreamService::_isStreamingActive = false;

static const char* _stream_contentType = "multipart/x-mixed-replace;boundary=" PART_BOUNDARY;
static const char* _stream_boundary = "\r\n--" PART_BOUNDARY "\r\n";
static const char* _stream_part = "Content-Type: image/jpeg\r\nContent-Length: %u\r\n\r\n";

bool HttpStreamService::start() {
    if (_serverHandle != nullptr) {
        return true;
    }

    Serial.println("[HTTP] Inicializando servidor web de streaming de vídeo...");

    httpd_config_t config = HTTPD_DEFAULT_CONFIG();
    config.server_port = 80;
    
    // Otimização de hardware do ESP32-CAM: limita conexões para economizar SRAM
    config.max_open_sockets = 2;
    config.max_uri_handlers = 2;
    config.lru_purge_enable = true; // Libera conexões inativas se faltar socket

    httpd_uri_t streamUri = {
        .uri = "/stream",
        .method = HTTP_GET,
        .handler = streamHandler,
        .user_ctx = nullptr
    };

    esp_err_t err = httpd_start(&_serverHandle, &config);
    if (err == ESP_OK) {
        httpd_register_uri_handler(_serverHandle, &streamUri);
        Serial.println("[HTTP] Servidor de câmera ativo no endpoint http://<ip_dispositivo>/stream");
        return true;
    } else {
        Serial.printf("[HTTP ERROR] Falha ao iniciar servidor HTTP: 0x%x\n", err);
        _serverHandle = nullptr;
        return false;
    }
}

void HttpStreamService::stop() {
    if (_serverHandle != nullptr) {
        httpd_stop(_serverHandle);
        _serverHandle = nullptr;
        _isStreamingActive = false;
        Serial.println("[HTTP] Servidor de câmera encerrado.");
    }
}

bool HttpStreamService::isStreaming() {
    return _isStreamingActive;
}

esp_err_t HttpStreamService::streamHandler(httpd_req_t* req) {
    // Regra estrita de negócio: Capping de clientes de vídeo
    if (_isStreamingActive) {
        Serial.println("[HTTP] Conexão de streaming recusada: Já existe um cliente conectado.");
        return httpd_resp_send_err(req, HTTPD_403_FORBIDDEN, "Only one stream client allowed");
    }

    _isStreamingActive = true;
    Serial.println("[HTTP] Cliente de streaming de vídeo conectado.");

    camera_fb_t* fb = nullptr;
    esp_err_t res = ESP_OK;
    char part_buf[64];

    res = httpd_resp_set_type(req, _stream_contentType);
    if (res != ESP_OK) {
        _isStreamingActive = false;
        return res;
    }

    // Envia o primeiro boundary sem CRLF inicial (padrão RFC de multipart/x-mixed-replace)
    res = httpd_resp_send_chunk(req, "--" PART_BOUNDARY "\r\n", strlen("--" PART_BOUNDARY "\r\n"));

    while (res == ESP_OK) {
        // Coleta o frame de forma segura via abstração do CameraDriver
        fb = CameraDriver::acquireFrame();
        if (!fb) {
            Serial.println("[HTTP ERROR] Falha ao capturar frame do sensor");
            res = ESP_FAIL;
            break;
        }

        // Escreve boundary
        res = httpd_resp_send_chunk(req, _stream_boundary, strlen(_stream_boundary));
        
        if (res == ESP_OK) {
            // Escreve os headers do frame (tamanho do buffer jpeg)
            size_t hlen = snprintf(part_buf, sizeof(part_buf), _stream_part, fb->len);
            res = httpd_resp_send_chunk(req, part_buf, hlen);
        }
        
        if (res == ESP_OK) {
            // Escreve o buffer binário JPEG
            res = httpd_resp_send_chunk(req, (const char*)fb->buf, fb->len);
        }

        // SEMPRE devolve o framebuffer para evitar vazamento de memória gravíssimo na SPIRAM!
        CameraDriver::releaseFrame(fb);
        fb = nullptr;

        // Limita a taxa de streaming para ~25 FPS para liberar a CPU para cuidar do mTLS/MQTT
        vTaskDelay(pdMS_TO_TICKS(40));
    }

    // Cleanup redundante de emergência caso haja queda súbita do soquete TCP
    if (fb) {
        CameraDriver::releaseFrame(fb);
        fb = nullptr;
    }

    _isStreamingActive = false;
    Serial.println("[HTTP] Cliente de streaming desconectado.");
    return res;
}

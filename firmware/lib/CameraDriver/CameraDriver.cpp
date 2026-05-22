#include "CameraDriver.h"

bool CameraDriver::_initialized = false;

bool CameraDriver::init() {
    if (_initialized) {
        return true;
    }

    Serial.println("[CAMERA] Executando ciclo de hard-reset elétrico da câmera (Power-Down)...");
    
    // Hard-Reset elétrico físico para limpar registradores da OV3660
    pinMode(CAM_PIN_PWDN, OUTPUT);
    digitalWrite(CAM_PIN_PWDN, HIGH); // Coloca em Power-Down
    delay(100);
    digitalWrite(CAM_PIN_PWDN, LOW);  // Liga o sensor novamente (Power-Up)
    delay(150);                       // Tempo crítico para estabilização interna da OV3660

    camera_config_t config;
    config.ledc_channel = LEDC_CHANNEL_0;
    config.ledc_timer = LEDC_TIMER_0;
    config.pin_d0 = CAM_PIN_D0;
    config.pin_d1 = CAM_PIN_D1;
    config.pin_d2 = CAM_PIN_D2;
    config.pin_d3 = CAM_PIN_D3;
    config.pin_d4 = CAM_PIN_D4;
    config.pin_d5 = CAM_PIN_D5;
    config.pin_d6 = CAM_PIN_D6;
    config.pin_d7 = CAM_PIN_D7;
    config.pin_xclk = CAM_PIN_XCLK;
    config.pin_pclk = CAM_PIN_PCLK;
    config.pin_vsync = CAM_PIN_VSYNC;
    config.pin_href = CAM_PIN_HREF;
    config.pin_sccb_sda = CAM_PIN_SIOD;
    config.pin_sccb_scl = CAM_PIN_SIOC;
    config.pin_pwdn = CAM_PIN_PWDN;
    config.pin_reset = CAM_PIN_RESET;
    
    // Frequência primária a 15 MHz (recomendação industrial para OV3660)
    config.xclk_freq_hz = 15000000;
    config.pixel_format = PIXFORMAT_JPEG;

    // Configuração robusta baseada em detecção dinâmica de PSRAM
    if (psramFound()) {
        Serial.println("[CAMERA] PSRAM ativa! Configurando buffers em SPIRAM.");
        config.frame_size = FRAMESIZE_VGA;   // 640x480 (Excelente relação performance/banda)
        config.jpeg_quality = 12;            // Excelente qualidade e compressão balanceada
        config.fb_count = 2;                 // Duplo buffering para evitar tearing no streaming
#if ESP_ARDUINO_VERSION >= ESP_ARDUINO_VERSION_VAL(2, 0, 0)
        config.fb_location = CAMERA_FB_IN_PSRAM; // Aloca explicitamente os framebuffers na PSRAM
#endif
    } else {
        Serial.println("[CAMERA WARNING] PSRAM não encontrada! Reduzindo recursos para SRAM.");
        config.frame_size = FRAMESIZE_CIF;   // Menor resolução para evitar OOM
        config.jpeg_quality = 15;            // Maior compressão
        config.fb_count = 1;                 // Single buffering apenas
    }

    // Inicializa o driver da câmera da Espressif com fallback automático de clock
    esp_err_t err = esp_camera_init(&config);
    if (err != ESP_OK) {
        Serial.printf("[CAMERA WARNING] Falha no esp_camera_init a 15MHz (Erro: 0x%x). Tentando fallback dinâmico a 10MHz...\n", err);
        
        // Tenta fallback dinâmico para 10MHz
        config.xclk_freq_hz = 10000000;
        err = esp_camera_init(&config);
        
        if (err != ESP_OK) {
            Serial.printf("[CAMERA ERROR] Falha fatal no fallback para 10MHz (Erro: 0x%x). Câmera desativada.\n", err);
            _initialized = false;
            return false;
        }
        Serial.println("[CAMERA] Câmera inicializada com sucesso em modo de fallback (10MHz)!");
    } else {
        Serial.println("[CAMERA] Câmera inicializada com sucesso a 15MHz!");
    }

    sensor_t* s = esp_camera_sensor_get();
    if (s) {
        // Correções adicionais de estabilização do sensor OV3660
        s->set_brightness(s, 0); // Neutro
        s->set_contrast(s, 0);
        s->set_saturation(s, 0);
        
        if (s->id.PID == OV3660_PID) {
            Serial.println("[CAMERA] Sensor OV3660 calibrado com perfis de fábrica.");
        }
    }

    _initialized = true;
    return true;
}

camera_fb_t* CameraDriver::acquireFrame() {
    if (!_initialized) {
        return nullptr;
    }
    return esp_camera_fb_get();
}

void CameraDriver::releaseFrame(camera_fb_t* fb) {
    if (fb) {
        esp_camera_fb_return(fb);
    }
}

bool CameraDriver::isReady() {
    return _initialized;
}

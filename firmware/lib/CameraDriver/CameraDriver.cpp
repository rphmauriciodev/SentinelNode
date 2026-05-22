#include "CameraDriver.h"

bool CameraDriver::_initialized = false;

bool CameraDriver::init() {
    if (_initialized) {
        return true;
    }

    Serial.println("[CAMERA] Inicializando sensor de câmera OV3660...");

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
    
    // Frequência de clock de 15 MHz para máxima estabilidade do sensor OV3660 no Linux/ESP32-CAM
    config.xclk_freq_hz = 15000000;
    config.pixel_format = PIXFORMAT_JPEG;

    // Configuração robusta baseada em detecção dinâmica de PSRAM
    if (psramFound()) {
        Serial.println("[CAMERA] PSRAM Hardware detectada e ativa! Alocando buffers em SPIRAM.");
        config.frame_size = FRAMESIZE_VGA;   // 640x480
        config.jpeg_quality = 12;            // Excelente qualidade e compressão balanceada
        config.fb_count = 2;                 // Duplo buffering para streaming fluído sem tearing
    } else {
        Serial.println("[CAMERA WARNING] PSRAM não encontrada! Alocando buffers limitados em SRAM.");
        config.frame_size = FRAMESIZE_CIF;   // Resolução menor para evitar OOM (Out Of Memory)
        config.jpeg_quality = 16;            // Maior compressão
        config.fb_count = 1;                 // Single buffer apenas
    }

    // Inicializa o driver da câmera da Espressif
    esp_err_t err = esp_camera_init(&config);
    if (err != ESP_OK) {
        Serial.printf("[CAMERA ERROR] Falha no esp_camera_init: 0x%x\n", err);
        _initialized = false;
        return false;
    }

    sensor_t* s = esp_camera_sensor_get();
    if (s) {
        // Correções adicionais de estabilização do sensor
        s->set_brightness(s, 0); // Neutro
        s->set_contrast(s, 0);
        s->set_saturation(s, 0);
        
        // Se for OV3660 especificamente, podemos aplicar correções adicionais se necessário
        if (s->id.PID == OV3660_PID) {
            Serial.println("[CAMERA] Sensor OV3660 identificado e calibrado.");
        }
    }

    Serial.println("[CAMERA] Câmera inicializada com sucesso!");
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

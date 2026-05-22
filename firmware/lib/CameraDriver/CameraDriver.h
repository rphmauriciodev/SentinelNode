#ifndef CAMERA_DRIVER_H
#define CAMERA_DRIVER_H

#include <Arduino.h>
#include "esp_camera.h"

// Definição dos pinos da câmera para o modelo AI Thinker ESP32-CAM
#define CAM_PIN_PWDN     32
#define CAM_PIN_RESET    -1
#define CAM_PIN_XCLK      0
#define CAM_PIN_SIOD     26
#define CAM_PIN_SIOC     27
#define CAM_PIN_D7       35
#define CAM_PIN_D6       34
#define CAM_PIN_D5       39
#define CAM_PIN_D4       36
#define CAM_PIN_D3       21
#define CAM_PIN_D2       19
#define CAM_PIN_D1       18
#define CAM_PIN_D0        5
#define CAM_PIN_VSYNC    25
#define CAM_PIN_HREF     23
#define CAM_PIN_PCLK     22

class CameraDriver {
public:
    // Inicializa a câmera. Retorna true se houver sucesso.
    static bool init();
    
    // Captura um frame buffer. Deve ser devolvido com releaseFrame.
    static camera_fb_t* acquireFrame();
    
    // Libera o frame buffer e limpa a memória associada.
    static void releaseFrame(camera_fb_t* fb);
    
    // Verifica se a câmera foi devidamente inicializada.
    static bool isReady();

private:
    static bool _initialized;
};

#endif // CAMERA_DRIVER_H

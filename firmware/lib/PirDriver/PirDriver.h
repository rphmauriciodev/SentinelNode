#ifndef PIR_DRIVER_H
#define PIR_DRIVER_H

#include <Arduino.h>

class PirDriver {
public:
    // Inicializa o sensor PIR no pino especificado. Default é GPIO 13.
    static void init(uint8_t pin = 13);
    
    // Verifica de forma não-bloqueante se o sensor detectou movimento.
    // Retorna true uma única vez por evento detectado (auto-reset) após validação.
    static bool hasTriggered();
    
    // Lê o estado lógico instantâneo do sensor (HIGH ou LOW)
    static bool readStatus();

private:
    static uint8_t _pin;
    static volatile bool _rawTrigger;
    static volatile uint64_t _lastInterruptTime;
    
    static bool _isValidating;
    static unsigned long _validationStart;
    static const unsigned long VALIDATION_WINDOW = 80; // Janela de 80ms para filtrar EMI do WiFi
    static bool _validatedTrigger;
    
    // Rotina de Serviço de Interrupção executada diretamente na IRAM do ESP32 (segura para IRAM)
    static void IRAM_ATTR pirISR();
};

#endif // PIR_DRIVER_H

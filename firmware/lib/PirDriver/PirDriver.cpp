#include "PirDriver.h"
#include <esp_timer.h>

uint8_t PirDriver::_pin = 13;
volatile bool PirDriver::_rawTrigger = false;
volatile uint64_t PirDriver::_lastInterruptTime = 0;

bool PirDriver::_isValidating = false;
unsigned long PirDriver::_validationStart = 0;
bool PirDriver::_validatedTrigger = false;

void PirDriver::init(uint8_t pin) {
    _pin = pin;
    
    // Configura o pino como entrada. NOTA: O GPIO13 possui um resistor de pull-up
    // físico de 10k na placa ESP32-CAM. Usar INPUT comum.
    pinMode(_pin, INPUT);
    
    // Associa o pino físico à interrupção IRAM chamando pirISR na borda de subida (RISING)
    attachInterrupt(digitalPinToInterrupt(_pin), pirISR, RISING);
    
    Serial.printf("[PIR] Driver de sensor PIR carregado no pino GPIO %d com interrupções ativas.\n", _pin);
}

void IRAM_ATTR PirDriver::pirISR() {
    // esp_timer_get_time() é um temporizador de 64 bits seguro para execução dentro de ISRs
    uint64_t now = esp_timer_get_time() / 1000; // Milissegundos
    
    // Filtro contra ruído mecânico/térmico (debounce físico primário de 150ms)
    if (now - _lastInterruptTime > 150) {
        _rawTrigger = true;
        _lastInterruptTime = now;
    }
}

bool PirDriver::hasTriggered() {
    unsigned long now = millis();

    // Se houve uma interrupção física (borda de subida)
    if (_rawTrigger) {
        _rawTrigger = false;
        
        // Só inicia a validação temporal se o pino de fato estiver em HIGH agora
        if (digitalRead(_pin) == HIGH && !_isValidating) {
            _isValidating = true;
            _validationStart = now;
        }
    }

    // Máquina de estados do filtro temporal de glitch (não-bloqueante)
    if (_isValidating) {
        // Se o pino cair para LOW antes de completar a janela, é considerado ruído (EMI / Glitch)
        if (digitalRead(_pin) == LOW) {
            _isValidating = false;
            // Log silencioso ou debug
        } else if (now - _validationStart >= VALIDATION_WINDOW) {
            // Sustentou HIGH estável durante toda a janela de 80ms!
            _isValidating = false;
            _validatedTrigger = true;
            Serial.printf("[PIR FILTER] Sinal sustentado de movimento validado com sucesso (Janela: %dms).\n", VALIDATION_WINDOW);
        }
    }

    // Padrão seguro de consumo do gatilho validado
    if (_validatedTrigger) {
        _validatedTrigger = false;
        return true;
    }
    
    return false;
}

bool PirDriver::readStatus() {
    return digitalRead(_pin) == HIGH;
}

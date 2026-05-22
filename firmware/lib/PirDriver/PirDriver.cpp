#include "PirDriver.h"

uint8_t PirDriver::_pin = 13;
volatile bool PirDriver::_triggered = false;
volatile unsigned long PirDriver::_lastInterruptTime = 0;

void PirDriver::init(uint8_t pin) {
    _pin = pin;
    pinMode(_pin, INPUT);
    
    // Associa o pino físico à interrupção IRAM chamando pirISR na borda de subida (RISING)
    attachInterrupt(digitalPinToInterrupt(_pin), pirISR, RISING);
    
    Serial.printf("[PIR] Driver de sensor PIR carregado no pino GPIO %d com interrupções ativas.\n", _pin);
}

void IRAM_ATTR PirDriver::pirISR() {
    unsigned long now = millis();
    // Filtro contra ruído mecânico/térmico (software debouncing de 200ms)
    if (now - _lastInterruptTime > 200) {
        _triggered = true;
        _lastInterruptTime = now;
    }
}

bool PirDriver::hasTriggered() {
    // Padrão Double-Check-Reset seguro contra race conditions
    if (_triggered) {
        _triggered = false;
        return true;
    }
    return false;
}

bool PirDriver::readStatus() {
    return digitalRead(_pin) == HIGH;
}

#include "WatchdogSystem.h"
#include <esp_task_wdt.h>

void WatchdogSystem::init(uint32_t timeoutSeconds) {
    Serial.printf("[WATCHDOG] Inicializando hardware Task WDT (%d segundos)...\n", timeoutSeconds);
    
    // Inicializa o WDT com o timeout desejado e ativa pânico (panic = true causará reboot instantâneo)
    esp_err_t err = esp_task_wdt_init(timeoutSeconds, true);
    if (err != ESP_OK) {
        Serial.printf("[WATCHDOG ERROR] Falha ao inicializar WDT: 0x%x\n", err);
        return;
    }
    
    // Associa a tarefa atual (loop do Arduino) ao watchdog
    err = esp_task_wdt_add(NULL);
    if (err == ESP_OK) {
        Serial.println("[WATCHDOG] Loop principal adicionado ao monitoramento do WDT.");
    } else {
        Serial.printf("[WATCHDOG ERROR] Falha ao adicionar loop ao WDT: 0x%x\n", err);
    }
}

void WatchdogSystem::feed() {
    esp_task_wdt_reset();
}

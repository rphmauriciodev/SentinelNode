#include "WifiService.h"

const char* WifiService::_ssid = nullptr;
const char* WifiService::_password = nullptr;
bool WifiService::_wasConnected = false;
bool WifiService::_justConnected = false;
unsigned long WifiService::_lastRetryTime = 0;

void WifiService::init(const char* ssid, const char* password) {
    _ssid = ssid;
    _password = password;
    _wasConnected = false;
    _justConnected = false;
    _lastRetryTime = 0;

    Serial.printf("[WIFI] Iniciando conexão de rede...\n");
    Serial.printf("[WIFI] SSID: %s\n", _ssid);
    
    // Configura o ESP32 como Station (cliente Wi-Fi) e garante baixo consumo de rádio se inativo
    WiFi.mode(WIFI_STA);
    WiFi.begin(_ssid, _password);

    // Boot Blocking com limite saudável de 10 segundos
    int attempts = 0;
    while (WiFi.status() != WL_CONNECTED && attempts < 20) {
        delay(500);
        Serial.print(".");
        attempts++;
    }

    if (WiFi.status() == WL_CONNECTED) {
        _wasConnected = true;
        _justConnected = true;
        IPAddress ip = WiFi.localIP();
        Serial.printf("\n[WIFI] Conectado no boot com sucesso! IP: %s\n", ip.toString().c_str());
    } else {
        Serial.println("\n[WIFI WARNING] Roteador não respondeu no boot. Entrando em modo de reconexão não-bloqueante...");
    }
}

void WifiService::handle() {
    unsigned long now = millis();
    wl_status_t status = WiFi.status();

    if (status == WL_CONNECTED) {
        if (!_wasConnected) {
            _wasConnected = true;
            _justConnected = true;
            IPAddress ip = WiFi.localIP();
            Serial.printf("[WIFI] Conectado e autenticado! IP: %s\n", ip.toString().c_str());
        }
    } else {
        if (_wasConnected) {
            Serial.println("[WIFI ERROR] Conexão Wi-Fi perdida abruptamente!");
            _wasConnected = false;
            _justConnected = false;
            _lastRetryTime = now; // Inicia a contagem de recuo
        }

        // Reconexão periódica não-bloqueante
        if (now - _lastRetryTime >= _retryInterval) {
            _lastRetryTime = now;
            Serial.println("[WIFI] Tentando reconectar ao ponto de acesso...");
            
            // Força reinicialização da conexão
            WiFi.disconnect();
            WiFi.begin(_ssid, _password);
        }
    }
}

bool WifiService::isConnected() {
    return WiFi.status() == WL_CONNECTED;
}

String WifiService::getIP() {
    if (WiFi.status() == WL_CONNECTED) {
        return WiFi.localIP().toString();
    }
    return "0.0.0.0";
}

bool WifiService::checkJustConnected() {
    if (_justConnected) {
        _justConnected = false; // Auto-clear trigger
        return true;
    }
    return false;
}

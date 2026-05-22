#include "MqttService.h"
#include "secrets.h"
#include "protocol.h"
#include <WiFi.h>

const char* MqttService::_deviceId = nullptr;
const char* MqttService::_defaultBrokerIp = nullptr;
const char* MqttService::_brokerToUse = nullptr;
int MqttService::_port = 8883;

WiFiClientSecure MqttService::_secureClient;
PubSubClient MqttService::_mqttClient(MqttService::_secureClient);

unsigned long MqttService::_lastRetryTime = 0;
unsigned long MqttService::_currentRetryInterval = MqttService::MIN_RETRY_INTERVAL;
bool MqttService::_wasConnected = false;

void MqttService::init(const char* deviceId, const char* defaultBrokerIp, int port) {
    _deviceId = deviceId;
    _defaultBrokerIp = defaultBrokerIp;
    _port = port;
    _lastRetryTime = 0;
    _currentRetryInterval = MIN_RETRY_INTERVAL;
    _wasConnected = false;
    
    setupSecureClient();
}

void MqttService::setupSecureClient() {
    IPAddress resolvedIp;
    
    if (WiFi.SSID() == "Wokwi-GUEST") {
        Serial.println("[MQTT] Ambiente Wokwi detectado. Usando IP local do gateway simulator.");
        _brokerToUse = "10.10.0.2";
        _secureClient.setCACert(nullptr); // Desativa TLS no simulador
        _mqttClient.setServer(_brokerToUse, _port);
    } else {
        Serial.println("[MQTT] Resolvendo mDNS para mauricio-notebook.local...");
        
        if (WiFi.hostByName("mauricio-notebook.local", resolvedIp) && resolvedIp.toString() != "0.0.0.0") {
            Serial.printf("[MQTT] mDNS resolvido com sucesso! Host: %s, IP: %s\n", 
                          "mauricio-notebook.local", resolvedIp.toString().c_str());
            _brokerToUse = "mauricio-notebook.local";
            _mqttClient.setServer(_brokerToUse, _port);
            
            // Carrega cadeia de certificados e chave privada para mTLS estrito
            _secureClient.setCACert(ca_cert);
            _secureClient.setCertificate(client_cert);
            _secureClient.setPrivateKey(client_key);
        } else {
            Serial.println("[MQTT WARNING] Falha ao resolver mDNS de mauricio-notebook.local. Usando IP estático com bypass de validação de hostname...");
            _brokerToUse = _defaultBrokerIp;
            _mqttClient.setServer(_brokerToUse, _port);
            
            // Bypass da validação estrita de hostname (devido ao IP direto), mas mantendo mTLS
            _secureClient.setInsecure();
            _secureClient.setCertificate(client_cert);
            _secureClient.setPrivateKey(client_key);
        }
    }
    
    // Configura tempo limite de keep alive estendido (60s)
    _mqttClient.setKeepAlive(60);
}

bool MqttService::connectBroker() {
    Serial.printf("[MQTT] Tentando estabelecer conexão TLS segura com broker %s...\n", _brokerToUse);
    
    if (_mqttClient.connect(_deviceId)) {
        Serial.println("[MQTT] Conexão segura estabelecida com sucesso!");
        _wasConnected = true;
        return true;
    } else {
        int state = _mqttClient.state();
        const char* reason = "Desconhecido";
        switch (state) {
            case -4: reason = "Connection Timeout (Sem resposta do broker)"; break;
            case -3: reason = "Connection Lost (Conexão caiu após estabelecida)"; break;
            case -2: reason = "Connect Failed (Falha ao abrir socket TCP)"; break;
            case -1: reason = "Disconnected (Cliente desconectado)"; break;
            case 1:  reason = "Bad Protocol Version (Protocolo incompatível)"; break;
            case 2:  reason = "Identifier Rejected (ID do cliente recusado)"; break;
            case 3:  reason = "Server Unavailable (Broker indisponível)"; break;
            case 4:  reason = "Bad Credentials (Usuário ou senha incorretos)"; break;
            case 5:  reason = "Unauthorized (Sem autorização)"; break;
        }
        Serial.printf("[MQTT ERROR] Falha na conexão TLS. Estado: %d [%s]\n", state, reason);
        return false;
    }
}

void MqttService::handle() {
    // Só processa se o Wi-Fi estiver online
    if (WiFi.status() != WL_CONNECTED) {
        if (_wasConnected) {
            _wasConnected = false;
            Serial.println("[MQTT] Conexão com o broker suspensa devido a queda do Wi-Fi.");
        }
        return;
    }

    unsigned long now = millis();

    if (!_mqttClient.connected()) {
        if (_wasConnected) {
            Serial.println("[MQTT] Conexão com o broker perdida!");
            _wasConnected = false;
            _lastRetryTime = now; // Inicia contagem
            _currentRetryInterval = MIN_RETRY_INTERVAL; // Reseta backoff inicial
        }

        // Reconexão periódica não-bloqueante com backoff exponencial
        if (now - _lastRetryTime >= _currentRetryInterval) {
            _lastRetryTime = now;
            
            Serial.printf("[MQTT] Tentando reconectar (Intervalo atual: %lu segundos)...\n", _currentRetryInterval / 1000);
            
            // Tenta restabelecer segurança antes de discar se houve transição de rede
            if (_brokerToUse == nullptr) {
                setupSecureClient();
            }
            
            if (connectBroker()) {
                _currentRetryInterval = MIN_RETRY_INTERVAL; // Reseta ao conectar com sucesso
            } else {
                // Dobra o intervalo até o limite máximo de 5 minutos
                _currentRetryInterval = min(_currentRetryInterval * 2, MAX_RETRY_INTERVAL);
            }
        }
    } else {
        _mqttClient.loop();
    }
}

bool MqttService::isConnected() {
    return _mqttClient.connected();
}

bool MqttService::sendHeartbeat(uint8_t status, uint8_t major, uint8_t minor, uint8_t patch, const char* streamUrl) {
    if (!isConnected()) {
        return false;
    }
    
    uint8_t buf[256];
    time_t now = time(nullptr);
    
    // Serialização em formato binário customizado usando o namespace do Protocolo
    int size = Protocol::encodeHeartbeat(buf, _deviceId, (uint64_t)now, status, major, minor, patch, streamUrl);
    
    char topic[128];
    snprintf(topic, sizeof(topic), "sentinel/devices/%s/heartbeat", _deviceId);
    
    if (_mqttClient.publish(topic, buf, size)) {
        Serial.printf("[MQTT] Pacote binário de Heartbeat publicado (%d bytes) no tópico: %s\n", size, topic);
        return true;
    } else {
        Serial.println("[MQTT ERROR] Falha ao publicar Heartbeat!");
        return false;
    }
}

bool MqttService::sendEvent(uint8_t eventType, const char* videoUrl) {
    if (!isConnected()) {
        return false;
    }
    
    uint8_t buf[256];
    time_t now = time(nullptr);
    
    // Serialização em formato binário customizado
    int size = Protocol::encodeEvent(buf, _deviceId, (uint64_t)now, eventType, videoUrl);
    
    char topic[128];
    snprintf(topic, sizeof(topic), "sentinel/devices/%s/events", _deviceId);
    
    if (_mqttClient.publish(topic, buf, size)) {
        Serial.printf("[MQTT] Alerta de evento binário publicado (%d bytes) no tópico: %s. URL de vídeo: %s\n", 
                      size, topic, videoUrl);
        return true;
    } else {
        Serial.println("[MQTT ERROR] Falha ao publicar Evento!");
        return false;
    }
}

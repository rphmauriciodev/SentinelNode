#ifndef MQTT_SERVICE_H
#define MQTT_SERVICE_H

#include <Arduino.h>
#include <WiFiClientSecure.h>
#include <PubSubClient.h>

class MqttService {
public:
    // Configura os certificados mTLS e estabelece o endereço do Broker
    static void init(const char* deviceId, const char* defaultBrokerIp, int port);
    
    // Executa a manutenção não-bloqueante da conexão e keep-alives
    static void handle();
    
    // Retorna true se estiver conectado e autenticado no broker
    static bool isConnected();
    
    // Envia o heartbeat binário do dispositivo seguindo a especificação de protocolo
    static bool sendHeartbeat(uint8_t status, uint8_t major, uint8_t minor, uint8_t patch, const char* streamUrl);
    
    // Envia o alerta de movimento binário seguindo a especificação de protocolo
    static bool sendEvent(uint8_t eventType, const char* videoUrl);

private:
    static void setupSecureClient();
    static bool connectBroker();

    static const char* _deviceId;
    static const char* _defaultBrokerIp;
    static const char* _brokerToUse;
    static int _port;
    
    static WiFiClientSecure _secureClient;
    static PubSubClient _mqttClient;
    
    static unsigned long _lastRetryTime;
    static unsigned long _currentRetryInterval;
    static const unsigned long MIN_RETRY_INTERVAL = 5000;   // 5 segundos
    static const unsigned long MAX_RETRY_INTERVAL = 300000; // 5 minutos (300 segundos)
    static bool _wasConnected;
};

#endif // MQTT_SERVICE_H

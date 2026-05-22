#ifndef WIFI_SERVICE_H
#define WIFI_SERVICE_H

#include <Arduino.h>
#include <WiFi.h>

class WifiService {
public:
    // Inicializa o serviço e inicia a primeira tentativa de conexão
    static void init(const char* ssid, const char* password);
    
    // Executa a máquina de estados não-bloqueante na loop principal
    static void handle();
    
    // Retorna true se estiver conectado
    static bool isConnected();
    
    // Obtém o endereço IP local em formato de string
    static String getIP();
    
    // Retorna se houve transição recente para conectado (lógica de trigger)
    static bool checkJustConnected();

private:
    static const char* _ssid;
    static const char* _password;
    static bool _wasConnected;
    static bool _justConnected;
    static unsigned long _lastRetryTime;
    static unsigned long _currentRetryInterval;
    static const unsigned long MIN_RETRY_INTERVAL = 5000;   // Mínimo de 5 segundos
    static const unsigned long MAX_RETRY_INTERVAL = 300000; // Máximo de 5 minutos (300 segundos)
};

#endif // WIFI_SERVICE_H

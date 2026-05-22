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
    static const unsigned long _retryInterval = 10000; // 10 segundos
};

#endif // WIFI_SERVICE_H
